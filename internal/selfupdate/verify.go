package selfupdate

import (
	"errors"
	"fmt"
)

const releasesPage = "https://github.com/Vonage/cloud-runtime-cli/releases"

// The reasons a release does not verify. Each one fills the failure text in verifyError.
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
)

// verifyError is how a release that cannot be trusted is reported, whether its signature failed or a download for
// it did. Building the text here keeps its capital letter and full stop clear of the error-string lint.
type verifyError struct {
	tag   string
	cause error
}

func (e *verifyError) Error() string {
	return "Cannot verify vcr " + e.tag + ": " + e.cause.Error() +
		". Nothing was changed. Download it from " + releasesPage + " and check it by hand."
}

func (e *verifyError) Unwrap() error { return e.cause }

// Verify checks checksums.txt against the signatures (keyed by keyid) strictly and returns the expected
// SHA-256 (hex) of the asset named asset. It requires the first line to be "# vcr <tag>".
func Verify(checksums []byte, sigs map[string][]byte, tag, asset string, keys []TrustedKey) (sha256hex string, err error) {
	sum, err := lookup(checksums, sigs, tag, asset, keys)
	if err != nil {
		return "", &verifyError{tag: tag, cause: err}
	}
	return sum, nil
}

func lookup(checksums []byte, sigs map[string][]byte, tag, asset string, keys []TrustedKey) (string, error) {
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

// verifySignatures accepts the list when the signature file of any trusted key verifies; the first valid one wins.
// A key without a file is skipped, and a bad file for one key does not matter while another key verifies. When
// every file fails, the failure of the first key in the list is the one reported.
func verifySignatures(checksums []byte, sigs map[string][]byte, keys []TrustedKey) error {
	var first error
	for _, k := range keys {
		armored, ok := sigs[k.ID]
		if !ok {
			continue
		}
		err := verifySSHSIG(armored, checksums, k.Key)
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
