//go:build darwin || freebsd || netbsd || openbsd || dragonfly

package scan

import "syscall"

// statfsNames extracts the filesystem type and source device names that
// BSD style Statfs_t (darwin included) carries inline.
func statfsNames(st *syscall.Statfs_t) (fsType, device string) {
	return cstr(st.Fstypename[:]), cstr(st.Mntfromname[:])
}

// cstr converts a NUL terminated int8 array (statfs name fields) to a Go
// string.
func cstr(b []int8) string {
	buf := make([]byte, 0, len(b))
	for _, c := range b {
		if c == 0 {
			break
		}
		buf = append(buf, byte(c))
	}
	return string(buf)
}
