package instance

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"vonage-cloud-runtime-cli/internal/platform"
)

const noTargetMessage = "failed to validate flags: must provide either 'id' flag or 'project-name' and 'instance-name' flags"

// fakeReader is an instanceReader that records its calls.
type fakeReader struct {
	byID     func(id string) (platform.Instance, error)
	byName   func(project, name string) (platform.Instance, error)
	idCalls  []string
	namePair [][2]string
}

func (f *fakeReader) GetInstance(_ context.Context, id string) (platform.Instance, error) {
	f.idCalls = append(f.idCalls, id)
	return f.byID(id)
}

func (f *fakeReader) GetInstanceByName(_ context.Context, project, name string) (platform.Instance, error) {
	f.namePair = append(f.namePair, [2]string{project, name})
	return f.byName(project, name)
}

func found(id string) func(string) (platform.Instance, error) {
	return func(string) (platform.Instance, error) { return platform.Instance{ID: id}, nil }
}

func foundByName(id string) func(string, string) (platform.Instance, error) {
	return func(string, string) (platform.Instance, error) { return platform.Instance{ID: id}, nil }
}

func TestValidate(t *testing.T) {
	tests := []struct {
		name   string
		target target
		valid  bool
	}{
		{"id only", target{id: "I1"}, true},
		{"both names", target{project: "p", instance: "n"}, true},
		{"id and names", target{id: "I1", project: "p", instance: "n"}, true},
		{"id and a project", target{id: "I1", project: "p"}, true},
		{"nothing", target{}, false},
		{"project only", target{project: "p"}, false},
		{"instance only", target{instance: "n"}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.target.validate()

			if tt.valid {
				require.NoError(t, err)
				return
			}
			require.EqualError(t, err, noTargetMessage)
		})
	}
}

func TestLookupByIDWinsOverNames(t *testing.T) {
	reader := &fakeReader{byID: found("I1"), byName: foundByName("N1")}

	inst, err := target{id: "I1", project: "p", instance: "n"}.lookup(t.Context(), reader, removeNameForm)

	require.NoError(t, err)
	assert.Equal(t, "I1", inst.ID)
	assert.Equal(t, []string{"I1"}, reader.idCalls)
	assert.Empty(t, reader.namePair)
}

func TestLookupByNamesAsksForTheNames(t *testing.T) {
	reader := &fakeReader{byID: found("I1"), byName: foundByName("N1")}

	inst, err := target{project: "p", instance: "n"}.lookup(t.Context(), reader, removeNameForm)

	require.NoError(t, err)
	assert.Equal(t, "N1", inst.ID)
	assert.Empty(t, reader.idCalls)
	assert.Equal(t, [][2]string{{"p", "n"}}, reader.namePair)
}

func TestLookupNotFoundIDWording(t *testing.T) {
	reader := &fakeReader{byID: func(string) (platform.Instance, error) {
		return platform.Instance{}, fmt.Errorf("x: %w", platform.ErrNotFound)
	}}

	_, err := target{id: "I1"}.lookup(t.Context(), reader, removeNameForm)

	require.EqualError(t, err, `instance with id="I1" could not be found or may have been deleted`)
	assert.NotErrorIs(t, err, platform.ErrNotFound)
}

func TestLookupNotFoundNamesWording(t *testing.T) {
	reader := &fakeReader{byName: func(string, string) (platform.Instance, error) {
		return platform.Instance{}, fmt.Errorf("x: %w", platform.ErrNotFound)
	}}
	tests := []struct {
		name     string
		nameForm string
		want     string
	}{
		{"remove", removeNameForm, `instance with project_name="p" and instance_name="n" could not be found or may have been deleted`},
		{"log", logNameForm, `instance with project="p" and instance="n" could not be found or may have been deleted`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := target{project: "p", instance: "n"}.lookup(t.Context(), reader, tt.nameForm)

			require.EqualError(t, err, tt.want)
		})
	}
}

func TestLookupPassesOtherErrorsThrough(t *testing.T) {
	boom := errors.New("boom")
	reader := &fakeReader{
		byID:   func(string) (platform.Instance, error) { return platform.Instance{}, boom },
		byName: func(string, string) (platform.Instance, error) { return platform.Instance{}, boom },
	}

	_, byID := target{id: "I1"}.lookup(t.Context(), reader, removeNameForm)
	_, byName := target{project: "p", instance: "n"}.lookup(t.Context(), reader, removeNameForm)

	require.ErrorIs(t, byID, boom)
	require.ErrorIs(t, byName, boom)
}
