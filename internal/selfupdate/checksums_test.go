package selfupdate

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// verifySigned verifies a list that was just signed with the harness's trusted key, so a test can shape
// checksums.txt freely and only the list can make the verification fail.
func verifySigned(t testing.TB, list, tag, asset string) (string, error) {
	t.Helper()
	priv := harnessTrustedKey()
	key := trustedKeyOf(priv)
	data := []byte(list)
	sig := signVariant(t, priv, validVariant(data))
	return Verify(data, map[string][]byte{key.ID: sig}, tag, asset, []TrustedKey{key})
}

func TestVerifyRejectsMissingEntry(t *testing.T) {
	list, good, trusted := fixture(t, "checksums.txt"), fixture(t, "good.sig"), fixtureKey(t, "trusted.pub")
	sigs := map[string][]byte{trusted.ID: good}

	for _, asset := range []string{"vcr_windows_amd64.tar.gz", "vcr_linux_amd64.tar", "", "checksums.txt", "VCR_LINUX_AMD64.TAR.GZ"} {
		t.Run("asset "+asset, func(t *testing.T) {
			_, err := Verify(list, sigs, testTag, asset, []TrustedKey{trusted})

			require.EqualError(t, err, frame(testTag, "checksums.txt has no entry for "+asset))
		})
	}
}

func TestVerifyRejectsTagMismatch(t *testing.T) {
	list, good, trusted := fixture(t, "checksums.txt"), fixture(t, "good.sig"), fixtureKey(t, "trusted.pub")

	t.Run("a validly signed list for another tag", func(t *testing.T) {
		_, err := Verify(list, map[string][]byte{trusted.ID: good}, "v9.9.8", linuxAsset, []TrustedKey{trusted})

		require.EqualError(t, err, frame("v9.9.8", reasonOtherTag))
	})

	entry := sumHex("linux") + "  " + linuxAsset + "\n"
	firstLines := []string{
		"# vcr v9.9.8",
		"# vcr v9.9.9-rc.1",
		"# vcr 9.9.9",
		"# vcr v9.9.9 ",
		"#vcr v9.9.9",
		"# vcr  v9.9.9",
		"# VCR v9.9.9",
		"# vcr",
		"",
		sumHex("first") + "  " + darwinAsset,
	}
	for _, first := range firstLines {
		t.Run("first line "+first, func(t *testing.T) {
			_, err := verifySigned(t, first+"\n"+entry, testTag, linuxAsset)

			require.EqualError(t, err, frame(testTag, reasonOtherTag))
		})
	}
	t.Run("an empty list", func(t *testing.T) {
		_, err := verifySigned(t, "", testTag, linuxAsset)

		require.EqualError(t, err, frame(testTag, reasonOtherTag))
	})
}

func TestVerifyRejectsMalformedChecksums(t *testing.T) {
	header := "# vcr " + testTag + "\n"
	linux := sumHex("linux") + "  " + linuxAsset + "\n"
	darwin := sumHex("darwin") + "  " + darwinAsset + "\n"

	t.Run("an ordinary list is accepted", func(t *testing.T) {
		got, err := verifySigned(t, header+linux+darwin, testTag, darwinAsset)

		require.NoError(t, err)
		assert.Equal(t, sumHex("darwin"), got)
	})

	t.Run("a CRLF first line is not the header", func(t *testing.T) {
		_, err := verifySigned(t, "# vcr "+testTag+"\r\n"+linux+darwin, testTag, linuxAsset)

		require.EqualError(t, err, frame(testTag, reasonOtherTag))
	})
	t.Run("a list with CRLF line ends throughout", func(t *testing.T) {
		_, err := verifySigned(t, strings.ReplaceAll(header+linux+darwin, "\n", "\r\n"), testTag, linuxAsset)

		require.EqualError(t, err, frame(testTag, reasonOtherTag))
	})

	tests := []struct{ name, list string }{
		{"a duplicated name", header + linux + darwin + linux},
		{"a duplicated name with another hash", header + linux + sumHex("other") + "  " + linuxAsset + "\n"},
		{"a duplicate of another asset", header + darwin + linux + darwin},
		{"a hash of 63 characters", header + sumHex("linux")[1:] + "  " + linuxAsset + "\n"},
		{"a hash of 65 characters", header + "0" + sumHex("linux") + "  " + linuxAsset + "\n"},
		{"upper-case hex", header + strings.ToUpper(sumHex("linux")) + "  " + linuxAsset + "\n"},
		{"one upper-case digit", header + "A" + sumHex("linux")[1:] + "  " + linuxAsset + "\n"},
		{"a character that is not hex", header + "g" + sumHex("linux")[1:] + "  " + linuxAsset + "\n"},
		{"one space as the separator", header + sumHex("linux") + " " + linuxAsset + "\n"},
		{"three spaces as the separator", header + sumHex("linux") + "   " + linuxAsset + "\n"},
		{"a tab as the separator", header + sumHex("linux") + "\t\t" + linuxAsset + "\n"},
		{"the sha256sum binary marker", header + sumHex("linux") + " *" + linuxAsset + "\n"},
		{"no name", header + sumHex("linux") + "  \n"},
		{"no separator", header + sumHex("linux") + "\n"},
		{"a name with a space inside", header + sumHex("linux") + "  vcr linux.tar.gz\n"},
		{"a name with a trailing space", header + sumHex("linux") + "  " + linuxAsset + " \n"},
		{"a name with a control character", header + sumHex("linux") + "  vcr\x01linux\n"},
		{"a blank line in the middle", header + linux + "\n" + darwin},
		{"a blank line at the end", header + linux + darwin + "\n"},
		{"a comment line after the first", header + "# note\n" + linux},
		{"CRLF line ends after the first", header + strings.ReplaceAll(linux+darwin, "\n", "\r\n")},
		{"a line of text", header + "not a checksum line\n"},
		{"a leading space", header + " " + sumHex("linux") + "  " + linuxAsset + "\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := verifySigned(t, tt.list, testTag, linuxAsset)

			require.EqualError(t, err, frame(testTag, reasonMalformed))
		})
	}
}

func TestVerifyAcceptsTheListShapesItAllows(t *testing.T) {
	header := "# vcr " + testTag + "\n"
	linux := sumHex("linux") + "  " + linuxAsset

	tests := []struct{ name, list string }{
		{"one final newline", header + linux + "\n"},
		{"no final newline", header + linux},
		{"the entry among others", header + sumHex("a") + "  vcr_a.tar.gz\n" + linux + "\n" + sumHex("b") + "  checksums.txt\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := verifySigned(t, tt.list, testTag, linuxAsset)

			require.NoError(t, err)
			assert.Equal(t, sumHex("linux"), got)
		})
	}
}
