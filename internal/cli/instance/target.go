package instance

import (
	"context"
	"errors"
	"fmt"

	"github.com/Vonage/cloud-runtime-cli/internal/platform"
)

// The not-found wording of the names form differs between the commands; the id form is the same for both.
const (
	removeNameForm = "instance with project_name=%q and instance_name=%q could not be found or may have been deleted"
	logNameForm    = "instance with project=%q and instance=%q could not be found or may have been deleted"
	idNotFoundForm = "instance with id=%q could not be found or may have been deleted"
)

// errNoTarget is what a command that needs an instance says when it was given neither an id nor both names.
var errNoTarget = errors.New("failed to validate flags: must provide either 'id' flag or 'project-name' and 'instance-name' flags")

// instanceReader is the part of *platform.Client that finds an instance.
type instanceReader interface {
	GetInstance(ctx context.Context, id string) (platform.Instance, error)
	GetInstanceByName(ctx context.Context, project, name string) (platform.Instance, error)
}

// target says which instance a command works on: by id, or by project and instance name.
type target struct{ id, project, instance string }

// validate accepts an id, or both names. The id wins when both are given, and the names are then not looked at.
func (t target) validate() error {
	if t.id != "" || (t.project != "" && t.instance != "") {
		return nil
	}
	return errNoTarget
}

// lookup reads the instance by id when there is one, else by names. A missing instance becomes a plain error:
// the id form for an id, otherwise nameForm (removeNameForm or logNameForm) filled with the project and the
// instance. Every other error is returned as it came.
func (t target) lookup(ctx context.Context, r instanceReader, nameForm string) (platform.Instance, error) {
	if t.id != "" {
		inst, err := r.GetInstance(ctx, t.id)
		if errors.Is(err, platform.ErrNotFound) {
			return platform.Instance{}, fmt.Errorf(idNotFoundForm, t.id)
		}
		return inst, err
	}
	inst, err := r.GetInstanceByName(ctx, t.project, t.instance)
	if errors.Is(err, platform.ErrNotFound) {
		return platform.Instance{}, fmt.Errorf(nameForm, t.project, t.instance)
	}
	return inst, err
}
