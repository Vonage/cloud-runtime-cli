package format

import (
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/stretchr/testify/require"

	"vonage-cloud-runtime-cli/pkg/api"
)

func Test_parseCapVersion(t *testing.T) {
	tests := []struct {
		name    string
		caps    string
		want    string
		wantErr error
	}{
		{
			name:    "happy-path-messages",
			caps:    "messages",
			want:    "v0.1",
			wantErr: nil,
		},
		{
			name:    "happy-path-messaging",
			caps:    "messaging",
			want:    "v0.1",
			wantErr: nil,
		},
		{
			name:    "happy-path-voice",
			caps:    "voice",
			want:    "v0",
			wantErr: nil,
		},
		{
			name:    "happy-path-rtc",
			caps:    "rtc",
			want:    "v0",
			wantErr: nil,
		},
		{
			name:    "happy-path-",
			caps:    "messaging-v1",
			want:    "v1",
			wantErr: nil,
		},
		{
			name:    "parse-error",
			caps:    "messages.v0.1",
			want:    "",
			wantErr: errors.New("invalid capability - make sure version is referenced correctly"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseCapVersion(tt.caps)
			if tt.wantErr != nil {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)

			require.Equal(t, tt.want, got)
		})
	}
}

func TestParseCapabilities(t *testing.T) {
	tests := []struct {
		name    string
		caps    []string
		want    api.Capabilities
		wantErr error
	}{
		{
			name:    "happy-path-messages",
			caps:    []string{"messages-v0.1"},
			want:    api.Capabilities{Messages: "v0.1"},
			wantErr: nil,
		},
		{
			name:    "happy-path-messaging",
			caps:    []string{"messaging-v0.1"},
			want:    api.Capabilities{Messages: "v0.1"},
			wantErr: nil,
		},
		{
			name:    "happy-path-voice",
			caps:    []string{"voice-v0"},
			want:    api.Capabilities{Voice: "v0"},
			wantErr: nil,
		},
		{
			name:    "happy-path-rtc",
			caps:    []string{"rtc-v0"},
			want:    api.Capabilities{RTC: "v0"},
			wantErr: nil,
		},
		{
			name:    "happy-path-",
			caps:    []string{"messaging-v1"},
			want:    api.Capabilities{Messages: "v1"},
			wantErr: nil,
		},
		{
			name:    "parse-error",
			caps:    []string{"messages.v0.1"},
			want:    api.Capabilities{},
			wantErr: errors.New("invalid capability - make sure version is referenced correctly"),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseCapabilities(tt.caps)
			if tt.wantErr != nil {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)

			require.Equal(t, tt.want, got)
		})
	}
}

func TestPrintUpdateMessage(t *testing.T) {
	ios, _, stdout, _ := iostreams.Test()
	version := "1.0.0"
	updateMessageChan := make(chan string)
	expectedOutput := fmt.Sprintf("\n\n%s %s → %s\nTo upgrade, run: %s\n",
		"A new release of vcr is available:",
		"1.0.0",
		"2.0.0",
		"vcr upgrade")

	go func() {
		updateMessageChan <- "2.0.0"
	}()

	PrintUpdateMessage(ios, version, updateMessageChan)

	require.Equal(t, expectedOutput, stdout.String())

	ios, _, _, stderr := iostreams.Test()
	updateMessageChan = make(chan string)
	errMessage := "Invalid release message"
	go func() {
		updateMessageChan <- errMessage
	}()

	PrintUpdateMessage(ios, version, updateMessageChan)

	require.Equal(t, fmt.Sprintf("\n\n%s\n", errMessage), stderr.String())

	ios, _, stdout, stderr = iostreams.Test()
	updateMessageChan = make(chan string)
	go func() {
		time.Sleep(1 * time.Second)
	}()

	PrintUpdateMessage(ios, version, updateMessageChan)

	require.Equal(t, "", stdout.String())
	require.Equal(t, "", stderr.String())
}

func TestGetAppOptions(t *testing.T) {
	apps := []api.ApplicationListItem{
		{ID: "1", Name: "App 1"},
		{ID: "2", Name: "App 2"},
		{ID: "3", Name: "App 3"},
	}

	expectedOptions := AppOptions{
		Labels: []string{"SKIP", "App 1 - (1)", "App 2 - (2)", "App 3 - (3)"},
		Lookup: map[string]string{
			"1": "App 1 - (1)",
			"2": "App 2 - (2)",
			"3": "App 3 - (3)",
		},
		IDLookup: map[string]string{
			"App 1 - (1)": "1",
			"App 2 - (2)": "2",
			"App 3 - (3)": "3",
		},
	}

	options := GetAppOptions(apps)

	require.Equal(t, expectedOptions.Labels, options.Labels)
	require.Equal(t, expectedOptions.Lookup, options.Lookup)
	require.Equal(t, expectedOptions.IDLookup, options.IDLookup)
}

func TestGetRuntimeOptions(t *testing.T) {
	runtimes := []api.Runtime{
		{Language: "go", Name: "Go", Comments: "Go comments"},
		{Language: "python", Name: "Python"},
		{Language: "java", Name: "Java", Comments: "Java comments"},
	}

	expectedOptions := RuntimeOptions{
		Labels: []string{"Go - (Go comments)", "Python", "Java - (Java comments)"},
		Lookup: map[string]string{
			"Go":     "Go - (Go comments)",
			"Python": "Python",
			"Java":   "Java - (Java comments)",
		},
		RuntimeLookup: map[string]string{
			"Go - (Go comments)":     "Go",
			"Python":                 "Python",
			"Java - (Java comments)": "Java",
		},
	}

	options := GetRuntimeOptions(runtimes)

	require.Equal(t, expectedOptions.Labels, options.Labels)
	require.Equal(t, expectedOptions.Lookup, options.Lookup)
	require.Equal(t, expectedOptions.RuntimeLookup, options.RuntimeLookup)
}

func TestGetRegionOptions(t *testing.T) {
	regions := []api.Region{
		{Name: "US East", Alias: "us-east"},
		{Name: "Europe", Alias: "europe"},
		{Name: "Asia Pacific", Alias: "apac"},
		{Name: "TEST - Europe Central", Alias: "test.euc1"},
		{Name: "AWS - Europe Central Test", Alias: "test.euc1"},
		{Name: "AWS - Europe Central", Alias: "test.euc1"},
		{Name: "AWS - Europe Central Test", Alias: "euc1"},
	}

	expectedOptions := RegionOptions{
		Labels: []string{"US East - (us-east)", "Europe - (europe)", "Asia Pacific - (apac)"},
		Lookup: map[string]string{
			"us-east": "US East - (us-east)",
			"europe":  "Europe - (europe)",
			"apac":    "Asia Pacific - (apac)",
		},
		AliasLookup: map[string]string{
			"US East - (us-east)":   "us-east",
			"Europe - (europe)":     "europe",
			"Asia Pacific - (apac)": "apac",
		},
	}

	options := GetRegionOptions(regions)

	require.Equal(t, expectedOptions.Labels, options.Labels)
	require.Equal(t, expectedOptions.Lookup, options.Lookup)
	require.Equal(t, expectedOptions.AliasLookup, options.AliasLookup)
}

func TestGetTemplateOptions(t *testing.T) {
	templateNames := []api.Product{
		{Name: "template1", ID: "1"},
		{Name: "template2", ID: "2"},
		{Name: "template3", ID: "3"},
	}

	expectedOptions := TemplateOptions{
		Labels: []string{"SKIP", "template1", "template2", "template3"},
		IDLookup: map[string]string{
			"template1": "1",
			"template2": "2",
			"template3": "3",
		},
	}

	options := GetTemplateOptions(templateNames)

	require.Equal(t, expectedOptions.Labels, options.Labels)
	require.Equal(t, expectedOptions.IDLookup, options.IDLookup)
}
