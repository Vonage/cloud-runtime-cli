package secret

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const nameError = `invalid secret name: must follow the regex format "^[a-zA-Z_]+[a-zA-Z0-9_]*$"`

func TestValidateName(t *testing.T) {
	for _, name := range []string{"A", "_x", "MY_KEY_2", "a1", "__", "a_B_c_9"} {
		t.Run("valid "+name, func(t *testing.T) {
			assert.NoError(t, validateName(name))
		})
	}

	invalid := map[string]string{
		"digit first":       "1abc",
		"dash":              "a-b",
		"space":             "a b",
		"dot":               "a.b",
		"empty":             "",
		"non-ASCII letter":  "é",
		"trailing newline":  "a\n",
		"leading newline":   "\na",
		"slash":             "a/b",
		"percent":           "a%b",
		"control character": "a\x07b",
	}
	for label, name := range invalid {
		t.Run("invalid "+label, func(t *testing.T) {
			err := validateName(name)

			require.Error(t, err)
			assert.EqualError(t, err, nameError)
		})
	}
}
