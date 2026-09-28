//go:build windows

package scan

import (
	"os"
	"strings"
	"syscall"
	"unsafe"
)

// fileAllocated on Windows falls back to the logical size: NTFS sparse
// files are rare among the caches scanned here and block counts are not
// exposed through os.FileInfo.
func fileAllocated(fi os.FileInfo) int64 { return fi.Size() }

var (
	modKernel32            = syscall.NewLazyDLL("kernel32.dll")
	procGetLogicalDrives   = modKernel32.NewProc("GetLogicalDrives")
	procGetDiskFreeSpaceEx = modKernel32.NewProc("GetDiskFreeSpaceExW")
	procGetDriveType       = modKernel32.NewProc("GetDriveTypeW")
	procGetVolumeInfo      = modKernel32.NewProc("GetVolumeInformationW")
)

// drive type constants returned by GetDriveTypeW.
const (
	driveUnknown   = 0
	driveNoRoot    = 1
	driveRemovable = 2
	driveFixed     = 3
	driveRemote    = 4
	driveCDROM     = 5
	driveRamDisk   = 6
)

// ListVolumes enumerates all drive letters via GetLogicalDrives and fills
// usage numbers with GetDiskFreeSpaceExW (user-available space, quota
// aware). Removable/network/CD drives are labelled accordingly; empty
// drives (no medium) are skipped.
func ListVolumes() []DiskVolume {
	var vols []DiskVolume

	ret, _, _ := procGetLogicalDrives.Call()
	mask := uint32(ret)
	for i := 0; i < 26; i++ {
		if mask&(1<<uint(i)) == 0 {
			continue
		}
		letter := rune('A' + i)
		root := string(letter) + `:\`

		dt, _, _ := procGetDriveType.Call(uintptr(unsafe.Pointer(syscall.StringToUTF16Ptr(root))))
		switch dt {
		case driveUnknown, driveNoRoot, driveCDROM:
			continue
		}

		var freeAvail, total, totalFree uint64
		rootUTF16, err := syscall.UTF16PtrFromString(root)
		if err != nil {
			continue
		}
		ok, _, _ := procGetDiskFreeSpaceEx.Call(
			uintptr(unsafe.Pointer(rootUTF16)),
			uintptr(unsafe.Pointer(&freeAvail)),
			uintptr(unsafe.Pointer(&total)),
			uintptr(unsafe.Pointer(&totalFree)),
		)
		if ok == 0 || total == 0 {
			continue // no medium / not ready
		}

		v := DiskVolume{
			Path:   root,
			Kind:   driveKind(dt),
			Device: `\\.\` + string(letter) + ":",
			Total:  int64(total),
			Free:   int64(freeAvail),
			Used:   int64(total - freeAvail),
		}
		v.Label, v.FSType = volumeInfo(rootUTF16)
		vols = append(vols, v)
	}

	return dedupeVolumes(vols)
}

func driveKind(dt uintptr) string {
	switch dt {
	case driveRemovable:
		return "removable"
	case driveRemote:
		return "remote"
	case driveRamDisk:
		return "ramdisk"
	default:
		return "fixed"
	}
}

// volumeInfo reads the volume label and filesystem name in one
// GetVolumeInformationW call.
func volumeInfo(root *uint16) (label, fsType string) {
	var nameBuf [256]uint16
	var fsBuf [64]uint16
	var serial, maxLen, flags uint32
	ok, _, _ := procGetVolumeInfo.Call(
		uintptr(unsafe.Pointer(root)),
		uintptr(unsafe.Pointer(&serial)),
		uintptr(unsafe.Pointer(&maxLen)),
		uintptr(unsafe.Pointer(&flags)),
		uintptr(unsafe.Pointer(&nameBuf[0])),
		uintptr(len(nameBuf)),
		uintptr(unsafe.Pointer(&fsBuf[0])),
		uintptr(len(fsBuf)),
	)
	if ok == 0 {
		return "", ""
	}
	return strings.TrimSpace(syscall.UTF16ToString(nameBuf[:])), syscall.UTF16ToString(fsBuf[:])
}
