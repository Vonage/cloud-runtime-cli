//go:build windows

package selfupdate

import (
	"errors"
	"os"
)

const staleSuffix = ".old"

// swap puts the finished file in place of target. A running executable cannot be overwritten on Windows but it can
// be renamed, so it is moved aside first and moved back if the new file cannot take its place. The file that was
// moved aside stays until the next upgrade removes it.
func swap(finished, target string) error {
	aside := target + staleSuffix
	_ = os.Remove(aside)
	if err := rename(target, aside); err != nil {
		return err
	}
	if err := rename(finished, target); err != nil {
		if restoreErr := rename(aside, target); restoreErr != nil {
			return errors.Join(err, restoreErr)
		}
		return err
	}
	return nil
}
