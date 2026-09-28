package scan

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestFormatSize(t *testing.T) {
	cases := []struct {
		in   int64
		want string
	}{
		{0, "0 B"},
		{512, "512 B"},
		{1024, "1.0 KB"},
		{1536, "1.5 KB"},
		{1024 * 1024, "1.0 MB"},
		{9_100_000_000, "8.5 GB"},
	}
	for _, c := range cases {
		if got := FormatSize(c.in); got != c.want {
			t.Errorf("FormatSize(%d) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestParseSize(t *testing.T) {
	cases := []struct {
		in   string
		want int64
	}{
		{"382B", 382},
		{"1.24kB", 1269},
		{"2 MB", 2 * 1024 * 1024},
		{"5.6GB", 6012954214},
		{"12GiB", 12884901888},
		{"1.2TB", 1319413953331},
		{"0B", 0},
		{"", 0},
		{"abc", 0},
		{"4.1GB (73%)", 4402341478},
	}
	for _, c := range cases {
		if got := ParseSize(c.in); got != c.want {
			t.Errorf("ParseSize(%q) = %d, want %d", c.in, got, c.want)
		}
	}
}

func TestDirSize(t *testing.T) {
	dir := t.TempDir()
	writeFile := func(p string, size int) {
		t.Helper()
		full := filepath.Join(dir, p)
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, make([]byte, size), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	writeFile("a.bin", 100)
	writeFile("sub/b.bin", 200)
	writeFile("sub/deep/c.bin", 300)

	size, files := DirSize(context.Background(), dir)
	if files != 3 {
		t.Fatalf("DirSize files = %d, want 3", files)
	}
	// Allocated semantics: each regular file counts as its on-disk blocks
	// (logical size rounded up to the filesystem block size).
	var want int64
	for _, p := range []string{"a.bin", filepath.Join("sub", "b.bin"), filepath.Join("sub", "deep", "c.bin")} {
		fi, err := os.Stat(filepath.Join(dir, p))
		if err != nil {
			t.Fatal(err)
		}
		want += fileAllocated(fi)
	}
	if size != want {
		t.Fatalf("DirSize = %d, want allocated sum %d", size, want)
	}

	// Missing dir must not panic or hang.
	if s, f := DirSize(context.Background(), filepath.Join(dir, "nope")); s != 0 || f != 0 {
		t.Fatalf("missing dir should be zero, got (%d,%d)", s, f)
	}
}

func TestParseOllamaList(t *testing.T) {
	out := `NAME                ID          SIZE      MODIFIED
deepseek-r1:14b     3d9f3a1b2c4d 9.0GB     3 days ago
llama3.2:3b         a80c4f17acd5 2.0 GB    18 months ago
qwen2.5-coder:1.5b  aa12bb34cc56 986MB     24 hours ago`
	models := parseOllamaList(out)
	if len(models) != 3 {
		t.Fatalf("want 3 models, got %d: %+v", len(models), models)
	}
	if models[0].name != "deepseek-r1:14b" || models[0].size != 9663676416 {
		t.Errorf("model[0] = %+v", models[0])
	}
	if models[1].name != "llama3.2:3b" || models[1].size != 2*1024*1024*1024 {
		t.Errorf("spaced size format not parsed: %+v", models[1])
	}
	if models[2].name != "qwen2.5-coder:1.5b" || models[2].size != 986*1024*1024 {
		t.Errorf("model[2] = %+v", models[2])
	}
}

func TestCleanDirValidation(t *testing.T) {
	env := &Env{Home: "/home/u", CacheDir: "/home/u/.cache", GOOS: "linux"}
	cl := NewCleaner(env, &nopWriter{})

	// Path escaping the allowed root must be refused.
	evil := &Target{
		ID: "x", Method: MethodDir, Path: "/etc", AllowedRoot: "/home/u/.cache",
		Size: 10, Available: true,
	}
	if res := cl.Clean(context.Background(), evil, false); res.OK {
		t.Fatal("cleaning outside allowed root must fail")
	}

	// The allowed root itself must be refused.
	root := &Target{
		ID: "x", Method: MethodDir, Path: "/home/u/.cache", AllowedRoot: "/home/u/.cache",
		Size: 10, Available: true,
	}
	if res := cl.Clean(context.Background(), root, false); res.OK {
		t.Fatal("cleaning the allowed root itself must fail")
	}
}

func TestCleanDirContents(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "inner"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "f.txt"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	cl := NewCleaner(&Env{}, &nopWriter{})
	res := cl.Clean(context.Background(), &Target{
		ID: "t", Method: MethodDirContents, Path: dir, AllowedRoot: filepath.Dir(dir),
		Size: 2, Available: true,
	}, false)
	if !res.OK {
		t.Fatalf("clean failed: %+v", res)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Fatalf("dir should be empty, has %d entries", len(entries))
	}
}

func TestWorkspaceScan(t *testing.T) {
	root := t.TempDir()
	mk := func(p string) {
		t.Helper()
		if err := os.MkdirAll(filepath.Join(root, p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(root, p, ".keep"), []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	mk(filepath.Join("proj-a", "node_modules", "vue"))
	mk(filepath.Join("proj-b", "dist"))
	mk(filepath.Join("proj-c", "app", "__pycache__"))
	mk(filepath.Join(".git", "objects"))

	env := &Env{GOOS: "linux", Home: root, CacheDir: filepath.Join(root, ".cache"),
		AppData: filepath.Join(root, ".local", "share"), Workspaces: []string{root}, NoExternal: true}

	p := &WorkspaceProvider{}
	var got []*Target
	p.Scan(context.Background(), env, func(t *Target) { got = append(got, t) })

	if len(got) != 1 {
		t.Fatalf("want 1 group, got %d", len(got))
	}
	group := got[0]
	paths := map[string]bool{}
	for _, c := range group.Items {
		paths[c.Path] = true
	}
	for _, want := range []string{
		filepath.Join(root, "proj-a", "node_modules"),
		filepath.Join(root, "proj-b", "dist"),
		filepath.Join(root, "proj-c", "app", "__pycache__"),
	} {
		if !paths[want] {
			t.Errorf("missing artifact %s in %+v", want, paths)
		}
	}
	for _, c := range group.Items {
		if filepath.Base(c.Path) == ".git" || filepath.Base(filepath.Dir(c.Path)) == ".git" {
			t.Errorf(".git must never be reported: %s", c.Path)
		}
	}
}

func TestFindTarget(t *testing.T) {
	tree := []*Target{{
		ID: "a", Items: []*Target{
			{ID: "a1"}, {ID: "a2", Items: []*Target{{ID: "a2x"}}},
		},
	}}
	if FindTarget(tree, "a2x") == nil {
		t.Fatal("nested child not found")
	}
	if FindTarget(tree, "zzz") != nil {
		t.Fatal("unknown id should not resolve")
	}
}

func TestRiskLabel(t *testing.T) {
	if RiskSafe.RiskLabel() != "安全" || RiskDangerous.RiskLabel() != "危险" {
		t.Fatal("unexpected labels")
	}
	if RiskOrder(RiskSafe) >= RiskOrder(RiskDangerous) {
		t.Fatal("risk order broken")
	}
}

type nopWriter struct{}

func (nopWriter) Write(p []byte) (int, error) { return len(p), nil }
