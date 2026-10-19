package upgrade

import (
	"bytes"
	"crypto/ed25519"
	"crypto/sha512"
	"encoding/base64"
	"encoding/binary"
	"errors"
	"fmt"
	"strings"
)

// Releases are signed with OpenSSH's `ssh-keygen -Y sign` (the SSHSIG format) using an ed25519 key. The namespace
// keeps these signatures from being valid for anything else.
const (
	armorBegin       = "-----BEGIN SSH SIGNATURE-----"
	armorEnd         = "-----END SSH SIGNATURE-----"
	sshsigMagic      = "SSHSIG"
	sshsigVersion    = 1
	keyTypeEd25519   = "ssh-ed25519"
	releaseNamespace = "vcr-release"
	releaseHash      = "sha512"
	lengthSize       = 4

	releasesPage = "https://github.com/Vonage/cloud-runtime-cli/releases"
)

// The reasons a release does not verify. Each one fills the failure text of verifyError.
var (
	errNoSignature  = errors.New("no signature for a release key")
	errNotSSHSig    = errors.New("the signature file is not a valid SSH signature")
	errUnsupported  = errors.New("the signature uses an unsupported format")
	errOtherPurpose = errors.New("the signature is for another purpose")
	errWrongKey     = errors.New("the signature was not made by the key it is filed under")
	errMismatch     = errors.New("the signature does not match checksums.txt")
	errOtherRelease = errors.New("checksums.txt is for another release")
	errMalformed    = errors.New("checksums.txt is malformed")
	errNoEntry      = errors.New("checksums.txt has no entry")

	// errWireType is a key or signature of a type other than ssh-ed25519.
	errWireType = errors.New("not an ssh-ed25519 value")
	// errWireShape is a string that is cut short, of the wrong size, or followed by bytes it should not have.
	errWireShape = errors.New("malformed SSH value")
)

// verifyError is how a release that cannot be trusted is reported, whether its signature failed or a download for
// it did. The text is built here, whole, so that it keeps its capital letter and full stop.
type verifyError struct {
	tag   string
	cause error
}

func (e *verifyError) Error() string {
	return "Cannot verify vcr " + e.tag + ": " + e.cause.Error() +
		". Nothing was changed. Download it from " + releasesPage + " and check it by hand."
}

// Is lets errors.Is reach the cause without exposing it to errors.As: an api.Error inside would be printed as an
// API failure instead of as a release that cannot be verified.
func (e *verifyError) Is(target error) bool { return errors.Is(e.cause, target) }

// verifyChecksums checks checksums.txt against the signature files (keyed by keyid) and returns the signed SHA-256,
// in hex, of the asset named asset. The first line of the list has to be "# vcr <tag>".
func verifyChecksums(checksums []byte, sigs map[string][]byte, tag, asset string, keys []trustedKey) (string, error) {
	sum, err := lookup(checksums, sigs, tag, asset, keys)
	if err != nil {
		return "", &verifyError{tag: tag, cause: err}
	}
	return sum, nil
}

func lookup(checksums []byte, sigs map[string][]byte, tag, asset string, keys []trustedKey) (string, error) {
	if err := verifySignatures(checksums, sigs, keys); err != nil {
		return "", err
	}
	entries, err := parseChecksums(checksums, tag)
	if err != nil {
		return "", err
	}
	sum, ok := entries[asset]
	if !ok {
		return "", fmt.Errorf("%w for %s", errNoEntry, asset)
	}
	return sum, nil
}

// verifySignatures accepts the list when the signature file of any trusted key verifies. A key without a file is
// skipped, and a bad file for one key does not matter while another key verifies. When every file fails, the
// failure of the first key in the list is the one reported.
func verifySignatures(checksums []byte, sigs map[string][]byte, keys []trustedKey) error {
	var first error
	for _, k := range keys {
		armored, ok := sigs[k.id]
		if !ok {
			continue
		}
		err := verifySSHSIG(armored, checksums, k.key)
		if err == nil {
			return nil
		}
		if first == nil {
			first = err
		}
	}
	if first == nil {
		return errNoSignature
	}
	return first
}

// sshsig is an SSHSIG signature once its layout has been checked.
type sshsig struct {
	key       [32]byte
	namespace string
	reserved  []byte
	hashAlg   string
	signature []byte
}

// verifySSHSIG checks that armored is a signature of msg made by the key want, in the release namespace.
func verifySSHSIG(armored, msg []byte, want [32]byte) error {
	blob, err := dearmor(armored)
	if err != nil {
		return err
	}
	sig, err := parseSSHSIG(blob)
	if err != nil {
		return err
	}
	// An SSHSIG carries its signer's key, so the embedded key must be the trusted one it is filed under.
	switch {
	case sig.key != want:
		return errWrongKey
	case sig.namespace != releaseNamespace:
		return errOtherPurpose
	case sig.hashAlg != releaseHash:
		return errUnsupported
	}
	if !ed25519.Verify(want[:], signedData(sig, msg), sig.signature) {
		return errMismatch
	}
	return nil
}

// dearmor decodes the base64 between the BEGIN and END lines, which may be wrapped at any width.
func dearmor(armored []byte) ([]byte, error) {
	text := strings.ReplaceAll(strings.TrimSpace(string(armored)), "\r\n", "\n")
	lines := strings.Split(text, "\n")
	if len(lines) < 3 || lines[0] != armorBegin || lines[len(lines)-1] != armorEnd {
		return nil, errNotSSHSig
	}
	blob, err := base64.StdEncoding.DecodeString(strings.Join(lines[1:len(lines)-1], ""))
	if err != nil {
		return nil, errNotSSHSig
	}
	return blob, nil
}

// parseSSHSIG reads the binary signature in the order of its layout and returns on the first defect:
//
//	"SSHSIG", uint32 version (1), string public key, string namespace, string reserved, string hash algorithm,
//	string signature, and nothing after it.
//
// The public key and the signature are each the string "ssh-ed25519" and a string of the key's or signature's
// bytes, with nothing after them.
func parseSSHSIG(blob []byte) (sshsig, error) {
	rest, ok := bytes.CutPrefix(blob, []byte(sshsigMagic))
	if !ok || len(rest) < lengthSize {
		return sshsig{}, errNotSSHSig
	}
	if binary.BigEndian.Uint32(rest) != sshsigVersion {
		return sshsig{}, errUnsupported
	}

	pubKey, rest, err := readString(rest[lengthSize:])
	if err != nil {
		return sshsig{}, errNotSSHSig
	}
	key, err := readEd25519(pubKey, ed25519.PublicKeySize)
	if err != nil {
		return sshsig{}, layoutFailure(err)
	}

	var fields [3][]byte // namespace, reserved, hash algorithm
	for i := range fields {
		if fields[i], rest, err = readString(rest); err != nil {
			return sshsig{}, errNotSSHSig
		}
	}

	sigBlob, rest, err := readString(rest)
	if err != nil {
		return sshsig{}, errNotSSHSig
	}
	signature, err := readEd25519(sigBlob, ed25519.SignatureSize)
	if err != nil {
		return sshsig{}, layoutFailure(err)
	}
	if len(rest) != 0 {
		return sshsig{}, errNotSSHSig
	}

	sig := sshsig{namespace: string(fields[0]), reserved: fields[1], hashAlg: string(fields[2]), signature: signature}
	copy(sig.key[:], key)
	return sig, nil
}

// layoutFailure is the reason a key or signature that readEd25519 refused is reported with: a type of another
// kind is an unsupported format, anything else is not an SSH signature.
func layoutFailure(err error) error {
	if errors.Is(err, errWireType) {
		return errUnsupported
	}
	return errNotSSHSig
}

// signedData is what ssh-keygen signs: "SSHSIG", then the namespace, reserved and hash algorithm strings, then the
// string holding the SHA-512 of msg.
func signedData(sig sshsig, msg []byte) []byte {
	digest := sha512.Sum512(msg)
	var buf bytes.Buffer
	buf.WriteString(sshsigMagic)
	writeString(&buf, []byte(sig.namespace))
	writeString(&buf, sig.reserved)
	writeString(&buf, []byte(sig.hashAlg))
	writeString(&buf, digest[:])
	return buf.Bytes()
}

// readString reads one SSH wire string, a big-endian uint32 length and that many bytes. The length is checked
// against what is left before anything is sliced, so no input can make it panic.
func readString(b []byte) (field, rest []byte, err error) {
	if len(b) < lengthSize {
		return nil, nil, errWireShape
	}
	n := binary.BigEndian.Uint32(b)
	body := b[lengthSize:]
	if int64(n) > int64(len(body)) {
		return nil, nil, errWireShape
	}
	return body[:n], body[n:], nil
}

// writeString appends s to buf as an SSH wire string.
func writeString(buf *bytes.Buffer, s []byte) {
	var length [lengthSize]byte
	binary.BigEndian.PutUint32(length[:], uint32(len(s))) //nolint:gosec // each string was read from a 32-bit length, or is a hash
	buf.Write(length[:])
	buf.Write(s)
}

// readEd25519 reads the blob that holds an ed25519 key or signature: the string "ssh-ed25519", then a string of
// size bytes, then nothing. It returns those bytes.
func readEd25519(blob []byte, size int) ([]byte, error) {
	kind, rest, err := readString(blob)
	if err != nil {
		return nil, err
	}
	if string(kind) != keyTypeEd25519 {
		return nil, errWireType
	}
	value, rest, err := readString(rest)
	if err != nil {
		return nil, err
	}
	if len(value) != size || len(rest) != 0 {
		return nil, errWireShape
	}
	return value, nil
}
