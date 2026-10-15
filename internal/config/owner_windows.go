//go:build windows

package config

import "os"

// ownedBy is always false on Windows, where permission bits say nothing about access, so nothing is tightened.
func ownedBy(_ os.FileInfo, _ int) bool {
	return false
}
