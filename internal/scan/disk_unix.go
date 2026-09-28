//go:build !windows

package scan

import (
	"os"
	"path/filepath"
	"runtime"
	"syscall"
)

// ListVolumes enumerates filesystem volumes:
//   - darwin: "/" plus everything under /Volumes (APFS volumes, DMGs, USB)
//   - linux: every whitelisted filesystem from /proc/self/mounts
//   - other unix: "/" as a safe fallback
//
// Free space uses Bavail (available to the unprivileged user), matching
// what df reports.
func ListVolumes() []DiskVolume {
	var vols []DiskVolume

	switch runtime.GOOS {
	case "darwin":
		vols = append(vols, darwinVolumes()...)
	case "linux":
		data, err := os.ReadFile("/proc/self/mounts")
		if err == nil {
			for _, m := range parseProcMounts(string(data)) {
				if v, ok := statfsVolume(m.point, m.device, m.fsType); ok {
					vols = append(vols, v)
				}
			}
		}
		if len(vols) == 0 {
			if v, ok := statfsVolume("/", "", ""); ok {
				vols = append(vols, v)
			}
		}
	default:
		if v, ok := statfsVolume("/", "", ""); ok {
			vols = append(vols, v)
		}
	}

	return dedupeVolumes(vols)
}

func fileExists(p string) bool {
	_, err := os.Stat(p)
	return err == nil
}

// fileAllocated returns the on-disk allocated size of a file. Sparse
// files (Docker.raw, APFS clones) report a huge logical size via
// FileInfo.Size() while occupying far fewer blocks — st_blocks*512 is
// the real footprint, matching what `du` reports.
func fileAllocated(fi os.FileInfo) int64 {
	if st, ok := fi.Sys().(*syscall.Stat_t); ok && st.Blocks > 0 {
		return int64(st.Blocks) * 512
	}
	return fi.Size()
}

func darwinVolumes() []DiskVolume {
	var vols []DiskVolume
	if v, ok := statfsVolume("/", "", ""); ok {
		vols = append(vols, v)
	}
	entries, err := os.ReadDir("/Volumes")
	if err != nil {
		return vols
	}
	for _, e := range entries {
		p := filepath.Join("/Volumes", e.Name())
		// Skip the firmlinked system volumes: same APFS container as "/".
		if e.Name() == "Macintosh HD" || e.Name() == "Preboot" ||
			e.Name() == "VM" || e.Name() == "Update" || e.Name() == "Recovery" ||
			e.Name() == "iOS Installer" {
			continue
		}
		if v, ok := statfsVolume(p, "", ""); ok {
			v.Kind = "volume"
			if v.Label == "" {
				v.Label = e.Name()
			}
			vols = append(vols, v)
		}
	}
	return vols
}

// statfsVolume fills usage numbers via statfs(2). device/fsType come from
// the caller (linux: /proc/mounts) or the syscall itself (darwin/bsd,
// which expose Fstypename/Mntfromname — see statfsNames accessors).
func statfsVolume(path, device, fsType string) (DiskVolume, bool) {
	var st syscall.Statfs_t
	if err := syscall.Statfs(path, &st); err != nil {
		return DiskVolume{}, false
	}
	bs := int64(st.Bsize)
	total := int64(st.Blocks) * bs
	free := int64(st.Bavail) * bs
	used := int64(st.Blocks-st.Bfree) * bs
	if total <= 0 {
		return DiskVolume{}, false
	}

	v := DiskVolume{
		Path:  path,
		Kind:  "fixed",
		Total: total,
		Used:  used,
		Free:  free,
	}
	sysFS, sysDev := statfsNames(&st)
	if fsType == "" {
		fsType = sysFS
	}
	if device == "" {
		device = sysDev
	}
	v.FSType, v.Device = fsType, device
	if free <= 0 {
		v.Kind = "readonly"
	}
	return v, true
}
