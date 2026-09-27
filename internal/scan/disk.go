package scan

import (
	"fmt"
	"sort"
	"strings"
)

// DiskVolume describes one filesystem volume (mount point or drive).
type DiskVolume struct {
	Path   string `json:"path"` // mount point (unix) or drive root (windows)
	Device string `json:"device,omitempty"`
	FSType string `json:"fsType,omitempty"`
	Kind   string `json:"kind"` // fixed | removable | remote | cdrom | volume
	Label  string `json:"label,omitempty"`
	Total  int64  `json:"total"`
	Used   int64  `json:"used"`
	Free   int64  `json:"free"` // available to the current user
}

// UsedPercent returns the used fraction in [0,100].
func (d DiskVolume) UsedPercent() float64 {
	if d.Total <= 0 {
		return 0
	}
	return float64(d.Used) / float64(d.Total) * 100
}

// linuxFSWhitelist keeps real filesystems out of pseudo ones when parsing
// /proc/mounts (proc, sysfs, cgroup, tmpfs... are RAM/kernel backed).
var linuxFSWhitelist = map[string]bool{
	"ext2": true, "ext3": true, "ext4": true,
	"xfs": true, "btrfs": true, "zfs": true, "f2fs": true,
	"vfat": true, "exfat": true, "ntfs": true, "ntfs3": true,
	"apfs": true, "hfsplus": true, "ufs": true, "ffs": true,
	"overlay": true, // WSL2 / container roots
	"fuse.sshfs": true, "fuseblk": true,
	"ecryptfs": true, "ramfs": false,
}

type mountEntry struct {
	device string
	point  string
	fsType string
}

// parseProcMounts parses the content of /proc/self/mounts (or /etc/mtab),
// keeping only whitelisted filesystems. Octal escapes like \040 (space)
// are decoded. Pure function — unit tested on every platform.
func parseProcMounts(content string) []mountEntry {
	var out []mountEntry
	for _, line := range strings.Split(content, "\n") {
		fields := strings.Fields(line)
		if len(fields) < 3 {
			continue
		}
		dev, point, fs := fields[0], decodeMountEscape(fields[1]), fields[2]
		if !linuxFSWhitelist[fs] {
			continue
		}
		// Loop / bind mounts of pseudo devices.
		if strings.HasPrefix(dev, "/dev/loop") || strings.HasPrefix(dev, "shm") {
			continue
		}
		out = append(out, mountEntry{device: dev, point: point, fsType: fs})
	}
	return out
}

// decodeMountEscape decodes octal escapes (\040 etc.) used in mountinfo.
func decodeMountEscape(s string) string {
	if !strings.ContainsRune(s, '\\') {
		return s
	}
	var b strings.Builder
	for i := 0; i < len(s); {
		if s[i] == '\\' && i+3 < len(s) {
			if v, ok := octal3(s[i+1:i+4]); ok {
				b.WriteByte(byte(v))
				i += 4
				continue
			}
		}
		b.WriteByte(s[i])
		i++
	}
	return b.String()
}

func octal3(s string) (int, bool) {
	v := 0
	for i := 0; i < 3; i++ {
		c := s[i]
		if c < '0' || c > '7' {
			return 0, false
		}
		v = v*8 + int(c-'0')
	}
	return v, true
}

// dedupeVolumes drops repeated devices (btrfs subvolumes, bind mounts,
// macOS double-listed volumes) keeping the first, most relevant entry.
// Root ("/" or "C:\\") always wins over other mount points of the same
// device.
func dedupeVolumes(vols []DiskVolume) []DiskVolume {
	byDevice := map[string]int{}
	rootIndex := map[string]bool{}
	out := make([]DiskVolume, 0, len(vols))
	for _, v := range vols {
		key := v.Device
		if key == "" {
			key = v.Path
		}
		if idx, ok := byDevice[key]; ok {
			if isRootVolume(v) && !isRootVolume(out[idx]) {
				out[idx] = v // prefer the root mount of this device
				rootIndex[key] = true
			}
			continue
		}
		byDevice[key] = len(out)
		if isRootVolume(v) {
			rootIndex[key] = true
		}
		out = append(out, v)
	}
	sort.SliceStable(out, func(i, j int) bool {
		ri, rj := isRootVolume(out[i]), isRootVolume(out[j])
		if ri != rj {
			return ri
		}
		if (out[i].Kind == "fixed") != (out[j].Kind == "fixed") {
			return out[i].Kind == "fixed"
		}
		return out[i].Path < out[j].Path
	})
	return out
}

func isRootVolume(v DiskVolume) bool {
	return v.Path == "/" || strings.HasPrefix(v.Path, "C:\\")
}

// DisksSummary renders a compact multi-line overview for the CLI.
func DisksSummary(vols []DiskVolume) string {
	var b strings.Builder
	for _, v := range vols {
		name := v.Path
		if v.Label != "" {
			name = fmt.Sprintf("%s (%s)", v.Path, v.Label)
		}
		fmt.Fprintf(&b, "磁盘 %s 总 %s,已用 %s (%.0f%%),剩余 %s\n",
			name, FormatSize(v.Total), FormatSize(v.Used), v.UsedPercent(), FormatSize(v.Free))
	}
	return strings.TrimRight(b.String(), "\n")
}
