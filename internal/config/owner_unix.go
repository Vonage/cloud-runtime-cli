//go:build !windows

package config

import (
	"os"
	"syscall"
)

// ownedBy reports whether the file belongs to the given user id.
func ownedBy(info os.FileInfo, uid int) bool {
	st, ok := info.Sys().(*syscall.Stat_t)
	return ok && int64(st.Uid) == int64(uid)
}
