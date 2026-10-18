//go:build !windows

package selfupdate

// swap puts the finished file in place of target in one rename. A process that is running the old file keeps it
// open and is not disturbed.
func swap(finished, target string) error { return rename(finished, target) }
