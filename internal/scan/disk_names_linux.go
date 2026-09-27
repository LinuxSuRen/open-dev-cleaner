//go:build linux

package scan

import "syscall"

// statfsNames returns empty names: Linux Statfs_t has no inline name
// arrays — device/fstype come from /proc/mounts instead.
func statfsNames(st *syscall.Statfs_t) (fsType, device string) {
	return "", ""
}
