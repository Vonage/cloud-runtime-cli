package manifest_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"vonage-cloud-runtime-cli/internal/manifest"
)

const invalidCapability = "invalid capability - make sure update is referenced correctly"

func TestParseCapabilities(t *testing.T) {
	tests := []struct {
		name    string
		caps    []string
		want    manifest.Capabilities
		wantErr string
	}{
		{name: "nil", caps: nil, want: manifest.Capabilities{}},
		{name: "empty list", caps: []string{}, want: manifest.Capabilities{}},

		{name: "messages has a default version", caps: []string{"messages"}, want: manifest.Capabilities{Messages: "v0.1"}},
		{name: "messaging has a default version", caps: []string{"messaging"}, want: manifest.Capabilities{Messages: "v0.1"}},
		{name: "voice has a default version", caps: []string{"voice"}, want: manifest.Capabilities{Voice: "v0"}},
		{name: "rtc has a default version", caps: []string{"rtc"}, want: manifest.Capabilities{RTC: "v0"}},
		{name: "video has a default version", caps: []string{"video"}, want: manifest.Capabilities{Video: "v0"}},
		{name: "network has a default version", caps: []string{"network"}, want: manifest.Capabilities{Network: "v0"}},

		{name: "messages with a version", caps: []string{"messages-v1"}, want: manifest.Capabilities{Messages: "v1"}},
		{name: "messaging with a version", caps: []string{"messaging-v2"}, want: manifest.Capabilities{Messages: "v2"}},
		{name: "voice with a version", caps: []string{"voice-v1"}, want: manifest.Capabilities{Voice: "v1"}},
		{name: "rtc with a version", caps: []string{"rtc-v1"}, want: manifest.Capabilities{RTC: "v1"}},
		{name: "video with a version", caps: []string{"video-v3"}, want: manifest.Capabilities{Video: "v3"}},
		{name: "verify with a version", caps: []string{"verify-v2"}, want: manifest.Capabilities{Verify: "v2"}},
		{name: "network with a version", caps: []string{"network-v4"}, want: manifest.Capabilities{Network: "v4"}},

		{name: "an empty version is accepted", caps: []string{"messages-"}, want: manifest.Capabilities{}},
		{name: "any text after the dash is the version", caps: []string{"voice-beta"}, want: manifest.Capabilities{Voice: "beta"}},
		{
			name: "several kinds together",
			caps: []string{"messages-v1", "voice", "rtc", "video-v2", "verify-v1", "network"},
			want: manifest.Capabilities{
				Messages: "v1", Voice: "v0", RTC: "v0", Video: "v2", Verify: "v1", Network: "v0",
			},
		},
		{
			name: "a later entry of the same kind overwrites an earlier one",
			caps: []string{"messages-v1", "messaging-v2"},
			want: manifest.Capabilities{Messages: "v2"},
		},
		{
			name: "the later entry wins even when it is the default",
			caps: []string{"voice-v2", "voice"},
			want: manifest.Capabilities{Voice: "v0"},
		},

		{name: "bare verify has no default version", caps: []string{"verify"}, wantErr: invalidCapability},
		{name: "voicemail matches the voice prefix", caps: []string{"voicemail"}, wantErr: invalidCapability},
		{name: "a version with a second dash", caps: []string{"voice-v1-beta"}, wantErr: invalidCapability},
		{name: "messages with only dashes", caps: []string{"messages--"}, wantErr: invalidCapability},
		{name: "messages with a suffix and no dash", caps: []string{"messages2"}, wantErr: invalidCapability},
		{
			name:    "an error discards what was parsed before it",
			caps:    []string{"messages-v1", "voice-v1", "rtc-v1-x"},
			wantErr: invalidCapability,
		},

		{name: "foo is ignored", caps: []string{"foo"}, want: manifest.Capabilities{}},
		{name: "sms is ignored", caps: []string{"sms"}, want: manifest.Capabilities{}},
		{name: "matching is case sensitive", caps: []string{"Voice"}, want: manifest.Capabilities{}},
		{name: "a leading space matches nothing", caps: []string{" voice"}, want: manifest.Capabilities{}},
		{name: "the empty string is ignored", caps: []string{""}, want: manifest.Capabilities{}},
		{
			name: "unknown names between known ones are skipped, even with dashes",
			caps: []string{"foo-v1-x", "voice", "bad-v1-x", "sms", ""},
			want: manifest.Capabilities{Voice: "v0"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := manifest.ParseCapabilities(tt.caps)

			if tt.wantErr != "" {
				require.EqualError(t, err, tt.wantErr)
				assert.Equal(t, manifest.Capabilities{}, got, "an error returns the zero value")
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}
