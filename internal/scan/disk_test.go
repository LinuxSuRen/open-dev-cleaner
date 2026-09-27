package scan

import (
	"strings"
	"testing"
)

func TestParseProcMounts(t *testing.T) {
	content := `/dev/disk3s1s1 / apfs rw 0 0
proc /proc proc rw 0 0
sysfs /sys sysfs rw 0 0
tmpfs /run tmpfs rw 0 0
/dev/sda2 /home ext4 rw 0 0
/dev/sda1 /boot vfat rw 0 0
/dev/mapper/data /var/lib/docker xfs rw 0 0
/dev/sdb1 /mnt/usb\040disk exfat rw 0 0
/dev/loop0 /snap/core22 squashfs ro 0 0
overlay / overlay2 rw 0 0
nas:/export /mnt/nas fuse.sshfs rw 0 0
`
	got := parseProcMounts(content)
	points := map[string]mountEntry{}
	for _, m := range got {
		points[m.point] = m
	}
	for _, want := range []string{"/", "/home", "/boot", "/var/lib/docker", "/mnt/usb disk", "/mnt/nas"} {
		if _, ok := points[want]; !ok {
			t.Errorf("mount %q missing in %+v", want, got)
		}
	}
	for _, unwanted := range []string{"/proc", "/sys", "/run", "/snap/core22"} {
		if _, ok := points[unwanted]; ok {
			t.Errorf("pseudo filesystem %q should be filtered", unwanted)
		}
	}
	if points["/home"].fsType != "ext4" || points["/home"].device != "/dev/sda2" {
		t.Errorf("bad entry: %+v", points["/home"])
	}
	if _, ok := points["/"]; !ok {
		t.Fatal("overlay root should be kept")
	}
}

func TestDedupeVolumes(t *testing.T) {
	vols := []DiskVolume{
		{Path: "/home", Device: "/dev/sda2", Total: 100, Used: 50, Free: 50, Kind: "fixed"},
		{Path: "/", Device: "/dev/mapper/root", Total: 200, Used: 100, Free: 100, Kind: "fixed"},
		{Path: "/var/lib/docker", Device: "/dev/sda2", Total: 100, Used: 50, Free: 50, Kind: "fixed"}, // same device as /home
		{Path: "/mnt/usb", Device: "/dev/sdb1", Total: 30, Used: 10, Free: 20, Kind: "removable"},
	}
	out := dedupeVolumes(vols)
	if len(out) != 3 {
		t.Fatalf("want 3 volumes after dedupe, got %d: %+v", len(out), out)
	}
	if out[0].Path != "/" {
		t.Errorf("root volume should sort first, got %+v", out)
	}
	// same-device duplicate: /home kept, /var/lib/docker dropped
	seen := map[string]bool{}
	for _, v := range out {
		seen[v.Path] = true
	}
	if seen["/var/lib/docker"] {
		t.Error("duplicate device mount should be dropped")
	}
}

func TestUsedPercent(t *testing.T) {
	if (DiskVolume{Total: 200, Used: 150}).UsedPercent() != 75 {
		t.Fatal("UsedPercent broken")
	}
	if (DiskVolume{}).UsedPercent() != 0 {
		t.Fatal("zero volume must be 0%")
	}
}

func TestDisksSummary(t *testing.T) {
	s := DisksSummary([]DiskVolume{{Path: "/", Label: "Macintosh HD", Total: 494 << 30, Used: 320 << 30, Free: 174 << 30}})
	if !strings.Contains(s, "/ (Macintosh HD)") || !strings.Contains(s, "剩余") || !strings.Contains(s, "174.0 GB") {
		t.Fatalf("summary format broken: %q", s)
	}
}

func TestListVolumesSmoke(t *testing.T) {
	vols := ListVolumes()
	if len(vols) == 0 {
		t.Skip("no volumes visible in this environment")
	}
	for _, v := range vols {
		if v.Path == "" || v.Total <= 0 {
			t.Errorf("bad volume: %+v", v)
		}
		if v.Free < 0 || v.Used < 0 || v.Used > v.Total {
			t.Errorf("usage out of range: %+v", v)
		}
	}
}
