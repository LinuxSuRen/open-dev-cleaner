package scan

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

// sparseCapable reports whether the current filesystem shrinks the block
// count of a file created by Truncate (hole punching).
func sparseCapable(t *testing.T, path string) bool {
	t.Helper()
	if err := os.Truncate(path, 64<<20); err != nil {
		return false
	}
	fi, err := os.Stat(path)
	if err != nil {
		return false
	}
	return fileAllocated(fi) < fi.Size()
}

// TestFileAllocatedSparse guards the Docker.raw over-counting bug: the
// logical size of a sparse file must not be reported as disk usage.
func TestFileAllocatedSparse(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("sparse block counts not exposed on windows")
	}
	dir := t.TempDir()
	p := filepath.Join(dir, "Docker.raw")

	// Real content: 8KB written → allocated ≈ 8KB.
	if err := os.WriteFile(p, make([]byte, 8192), 0o644); err != nil {
		t.Fatal(err)
	}
	fi, _ := os.Stat(p)
	if got := fileAllocated(fi); got < 4096 || got > 3*8192 {
		t.Errorf("allocated for 8KB file = %d, want ~8KB", got)
	}

	// Sparse: extend to 64MB logical without writing → allocated stays small.
	if !sparseCapable(t, p) {
		t.Skip("filesystem does not support sparse files")
	}
	fi, _ = os.Stat(p)
	if fi.Size() != 64<<20 {
		t.Fatalf("logical size = %d, want 64MB", fi.Size())
	}
	if got := fileAllocated(fi); got >= fi.Size() {
		t.Errorf("sparse file allocated = %d should be far below logical %d", got, fi.Size())
	}

	// DirSize must use the same allocated semantics.
	size, files := DirSize(context.Background(), dir)
	if files != 1 || size >= 64<<20 {
		t.Errorf("DirSize = (%d, %d), allocated size must stay below logical", size, files)
	}
}

// TestDockerDaemonDownSize verifies the macOS Docker.raw hint reports the
// allocated (not logical) size.
func TestDockerDaemonDownSize(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("darwin path layout")
	}
	home := t.TempDir()
	raw := filepath.Join(home, "Library", "Containers", "com.docker.docker", "Data", "vms", "0", "data", "Docker.raw")
	if err := os.MkdirAll(filepath.Dir(raw), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(raw, make([]byte, 8192), 0o644); err != nil {
		t.Fatal(err)
	}
	if !sparseCapable(t, raw) {
		t.Skip("filesystem does not support sparse files")
	}
	env := &Env{GOOS: "darwin", Home: home, NoExternal: true}
	tgt := (&DockerProvider{}).daemonDown(context.Background(), env, errTest)
	if tgt.Size >= 64<<20 {
		t.Errorf("daemonDown size = %d, want allocated (<64MB logical)", tgt.Size)
	}
	if tgt.Note == "" || tgt.Size == 0 {
		t.Errorf("note/size not filled: %+v", tgt)
	}
}

type errTestType struct{}

func (errTestType) Error() string { return "test: docker down" }

var errTest = errTestType{}
