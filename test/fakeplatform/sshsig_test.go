package fakeplatform

import (
	"bytes"
	"crypto/ed25519"
	"crypto/sha256"
	"crypto/sha512"
	"encoding/base64"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	testArmorBegin = "-----BEGIN SSH SIGNATURE-----\n"
	testArmorEnd   = "-----END SSH SIGNATURE-----\n"
)

// documentedKey derives a test key from its documented seed, independently of the package.
func documentedKey(label string) ed25519.PrivateKey {
	seed := sha256.Sum256([]byte(label))
	return ed25519.NewKeyFromSeed(seed[:])
}

var (
	trustedTestKey = documentedKey("vcr-contract-test-release-key")
	otherTestKey   = documentedKey("vcr-contract-test-other-key")
)

func publicOf(k ed25519.PrivateKey) ed25519.PublicKey {
	pub, _ := k.Public().(ed25519.PublicKey)
	return pub
}

// readSSHString reads one length-prefixed SSH wire string.
func readSSHString(b []byte) (s, rest []byte, err error) {
	if len(b) < 4 {
		return nil, nil, errors.New("truncated length")
	}
	n := binary.BigEndian.Uint32(b)
	if uint64(len(b)-4) < uint64(n) {
		return nil, nil, errors.New("truncated string")
	}
	return b[4 : 4+n], b[4+n:], nil
}

func appendSSHString(b, s []byte) []byte {
	b = binary.BigEndian.AppendUint32(b, uint32(len(s)))
	return append(b, s...)
}

// sshsig is a decoded SSHSIG blob.
type sshsig struct {
	version   uint32
	publicKey []byte
	namespace string
	reserved  []byte
	hash      string
	signature []byte
}

func dearmor(armored []byte) ([]byte, error) {
	text := string(armored)
	if !strings.HasPrefix(text, testArmorBegin) || !strings.HasSuffix(text, testArmorEnd) {
		return nil, errors.New("not an armored SSH signature")
	}
	body := strings.TrimSuffix(strings.TrimPrefix(text, testArmorBegin), testArmorEnd)
	return base64.StdEncoding.DecodeString(strings.ReplaceAll(body, "\n", ""))
}

func parseSSHSIG(blob []byte) (sshsig, error) {
	rest, ok := bytes.CutPrefix(blob, []byte("SSHSIG"))
	if !ok || len(rest) < 4 {
		return sshsig{}, errors.New("no SSHSIG magic")
	}
	sig := sshsig{version: binary.BigEndian.Uint32(rest)}
	rest = rest[4:]
	fields := make([][]byte, 5)
	for i := range fields {
		var err error
		if fields[i], rest, err = readSSHString(rest); err != nil {
			return sshsig{}, err
		}
	}
	if len(rest) != 0 {
		return sshsig{}, errors.New("trailing bytes")
	}
	sig.publicKey, sig.namespace, sig.reserved, sig.hash, sig.signature = fields[0], string(fields[1]), fields[2], string(fields[3]), fields[4]
	return sig, nil
}

// ed25519Payload unwraps a "ssh-ed25519" key or signature blob.
func ed25519Payload(blob []byte, size int) ([]byte, error) {
	typ, rest, err := readSSHString(blob)
	if err != nil {
		return nil, err
	}
	if string(typ) != "ssh-ed25519" {
		return nil, fmt.Errorf("type %q", typ)
	}
	payload, rest, err := readSSHString(rest)
	switch {
	case err != nil:
		return nil, err
	case len(rest) != 0:
		return nil, errors.New("trailing bytes")
	case len(payload) != size:
		return nil, fmt.Errorf("%d bytes, want %d", len(payload), size)
	}
	return payload, nil
}

// verifySSHSIG is a strict standard-library check of an armored signature over msg: it returns the signer's
// key and the namespace when the signature is valid.
func verifySSHSIG(armored, msg []byte) (ed25519.PublicKey, string, error) {
	blob, err := dearmor(armored)
	if err != nil {
		return nil, "", err
	}
	sig, err := parseSSHSIG(blob)
	if err != nil {
		return nil, "", err
	}
	if sig.version != 1 || sig.hash != "sha512" || len(sig.reserved) != 0 {
		return nil, "", fmt.Errorf("version %d, hash %q", sig.version, sig.hash)
	}
	pub, err := ed25519Payload(sig.publicKey, ed25519.PublicKeySize)
	if err != nil {
		return nil, "", fmt.Errorf("public key: %w", err)
	}
	raw, err := ed25519Payload(sig.signature, ed25519.SignatureSize)
	if err != nil {
		return nil, "", fmt.Errorf("signature: %w", err)
	}
	digest := sha512.Sum512(msg)
	signed := appendSSHString([]byte("SSHSIG"), []byte(sig.namespace))
	signed = appendSSHString(signed, sig.reserved)
	signed = appendSSHString(signed, []byte(sig.hash))
	signed = appendSSHString(signed, digest[:])
	if !ed25519.Verify(pub, signed, raw) {
		return nil, "", errors.New("the signature does not verify")
	}
	return pub, sig.namespace, nil
}

func TestPublicKeyLine(t *testing.T) {
	fields := strings.Split(TestPublicKey(), " ")
	require.Len(t, fields, 3)
	assert.Equal(t, "ssh-ed25519", fields[0])
	assert.Equal(t, "vcr-contract-test", fields[2])
	blob, err := base64.StdEncoding.DecodeString(fields[1])
	require.NoError(t, err)
	key, err := ed25519Payload(blob, ed25519.PublicKeySize)
	require.NoError(t, err)
	assert.Equal(t, []byte(publicOf(trustedTestKey)), key, "the trusted key from its documented seed")
}

func TestKeyIDIsSixteenHex(t *testing.T) {
	for _, k := range []ed25519.PrivateKey{trustedTestKey, otherTestKey} {
		id := KeyID(publicOf(k))
		assert.Regexp(t, `^[0-9a-f]{16}$`, id)
		sum := sha256.Sum256(publicOf(k))
		assert.Equal(t, hex.EncodeToString(sum[:8]), id)
	}
	assert.NotEqual(t, KeyID(publicOf(trustedTestKey)), KeyID(publicOf(otherTestKey)))
}

func TestSignatureStructure(t *testing.T) {
	msg := []byte("# vcr v2.6.1\n")
	armored := SignSSHSIG(trustedTestKey, "vcr-release", msg)
	blob, err := dearmor(armored)
	require.NoError(t, err)
	assert.Equal(t, "SSHSIG", string(blob[:6]))
	sig, err := parseSSHSIG(blob)
	require.NoError(t, err)
	assert.Equal(t, uint32(1), sig.version)
	assert.Equal(t, "vcr-release", sig.namespace)
	assert.Empty(t, sig.reserved)
	assert.Equal(t, "sha512", sig.hash)
	key, err := ed25519Payload(sig.publicKey, ed25519.PublicKeySize)
	require.NoError(t, err)
	assert.Equal(t, []byte(publicOf(trustedTestKey)), key)
	_, err = ed25519Payload(sig.signature, ed25519.SignatureSize)
	require.NoError(t, err)

	pub, namespace, err := verifySSHSIG(armored, msg)
	require.NoError(t, err)
	assert.Equal(t, publicOf(trustedTestKey), pub)
	assert.Equal(t, "vcr-release", namespace)
	_, _, err = verifySSHSIG(armored, []byte("# vcr v2.6.2\n"))
	assert.Error(t, err, "another message")
	assert.Equal(t, armored, SignSSHSIG(trustedTestKey, "vcr-release", msg), "ed25519 signatures are deterministic")

	other := SignSSHSIG(otherTestKey, "file", msg)
	pub, namespace, err = verifySSHSIG(other, msg)
	require.NoError(t, err)
	assert.Equal(t, publicOf(otherTestKey), pub)
	assert.Equal(t, "file", namespace)
}

func TestArmorWrapsAtSeventyColumns(t *testing.T) {
	for _, namespace := range []string{"vcr-release", "", strings.Repeat("n", 40)} {
		armored := string(SignSSHSIG(trustedTestKey, namespace, []byte("msg")))
		require.True(t, strings.HasPrefix(armored, testArmorBegin), armored)
		require.True(t, strings.HasSuffix(armored, "\n"+testArmorEnd), armored)
		lines := strings.Split(strings.TrimSuffix(armored, "\n"), "\n")
		body := lines[1 : len(lines)-1]
		require.Greater(t, len(body), 1)
		for i, line := range body {
			if i < len(body)-1 {
				assert.Len(t, line, 70, "line %d", i)
			} else {
				assert.NotEmpty(t, line)
				assert.LessOrEqual(t, len(line), 70)
			}
		}
	}
}

// keygenCannotCheck reports whether ssh-keygen's output says it does not know -Y.
func keygenCannotCheck(out string) bool {
	lower := strings.ToLower(out)
	return strings.Contains(lower, "illegal option") || strings.Contains(lower, "unknown option") ||
		strings.Contains(lower, "unsupported")
}

func TestSignatureAcceptedBySSHKeygen(t *testing.T) {
	keygen, err := exec.LookPath("ssh-keygen")
	if err != nil {
		t.Skip("ssh-keygen is not installed")
	}
	// The release that /releases/latest lists, fetched through its asset URLs.
	s := Start(t, Defaults())
	rel := latestRelease(t, s)
	msg := send(t, http.MethodGet, rel.urls["checksums.txt"], nil).body
	sig := send(t, http.MethodGet, rel.urls[sigNameOf(trustedTestKey)], nil).body
	_, _, err = verifySSHSIG(sig, msg)
	require.NoError(t, err, "the standard-library check")

	dir := t.TempDir()
	sigPath := filepath.Join(dir, "checksums.txt.sig")
	require.NoError(t, os.WriteFile(sigPath, sig, 0o600))
	run := func(input []byte, args ...string) (string, error) {
		cmd := exec.Command(keygen, append(append([]string{"-Y"}, args...), "-s", sigPath)...)
		cmd.Stdin = bytes.NewReader(input)
		out, err := cmd.CombinedOutput()
		return string(out), err
	}

	out, err := run(msg, "check-novalidate", "-n", "vcr-release")
	if err != nil && keygenCannotCheck(out) {
		t.Skipf("this ssh-keygen cannot check signatures: %s", out)
	}
	require.NoError(t, err, out)

	changed := bytes.Clone(msg)
	changed[len(changed)/2] ^= 0x01
	out, err = run(changed, "check-novalidate", "-n", "vcr-release")
	assert.Error(t, err, "one changed byte: %s", out)
	out, err = run(msg, "check-novalidate", "-n", "file")
	assert.Error(t, err, "another namespace: %s", out)

	allowed := filepath.Join(dir, "allowed_signers")
	key := strings.Join(strings.Split(TestPublicKey(), " ")[:2], " ")
	require.NoError(t, os.WriteFile(allowed, []byte(`vcr-release namespaces="vcr-release" `+key+"\n"), 0o600))
	out, err = run(msg, "verify", "-f", allowed, "-I", "vcr-release", "-n", "vcr-release")
	require.NoError(t, err, "the public key line names the signing key: %s", out)
}
