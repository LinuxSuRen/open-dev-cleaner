package scan

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// touchOld sets mtime far in the past to simulate a stale directory,
// including its direct children.
func touchOld(t *testing.T, path string, days int) {
	t.Helper()
	past := time.Now().AddDate(0, 0, -days)
	if err := os.Chtimes(path, past, past); err != nil {
		t.Fatal(err)
	}
	entries, err := os.ReadDir(path)
	if err != nil {
		return
	}
	for _, e := range entries {
		_ = os.Chtimes(filepath.Join(path, e.Name()), past, past)
	}
}

func TestVersionsProviderNvm(t *testing.T) {
	home := t.TempDir()
	versions := filepath.Join(home, ".nvm", "versions", "node")
	for _, v := range []string{"v22.18.0", "v20.16.0", "v17.0.0"} {
		dir := filepath.Join(versions, v)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "bin"), []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	touchOld(t, filepath.Join(versions, "v17.0.0"), 400) // stale
	touchOld(t, filepath.Join(versions, "v20.16.0"), 10) // recent
	if err := os.MkdirAll(filepath.Join(home, ".nvm", "alias"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(home, ".nvm", "alias", "default"), []byte("v22.18.0"), 0o644); err != nil {
		t.Fatal(err)
	}

	env := &Env{GOOS: "linux", Home: home, CacheDir: filepath.Join(home, ".cache"),
		AppData: filepath.Join(home, ".local", "share"), NoExternal: true}

	var groups []*Target
	(&VersionsProvider{}).Scan(context.Background(), env, func(t *Target) { groups = append(groups, t) })

	var nvm *Target
	for _, g := range groups {
		if g.ID == "versions:nvm" {
			nvm = g
		}
	}
	if nvm == nil {
		t.Fatalf("nvm group missing: %+v", groups)
	}
	if len(nvm.Items) != 3 {
		t.Fatalf("want 3 versions, got %d", len(nvm.Items))
	}
	byTitle := map[string]*Target{}
	for _, c := range nvm.Items {
		byTitle[c.Title] = c
	}
	if cur := byTitle["Node.js v22.18.0"]; cur == nil || cur.Note == "" {
		t.Errorf("current version note missing: %+v", cur)
	}
	if stale := byTitle["Node.js v17.0.0"]; stale == nil || stale.Note == "" {
		t.Errorf("stale version note missing: %+v", stale)
	}
	if recent := byTitle["Node.js v20.16.0"]; recent == nil || recent.Note != "" {
		t.Errorf("recent version should have no note: %+v", recent)
	}
	// deleting one version must be constrained to the versions root
	if res := NewCleaner(env, &nopWriter{}).Clean(context.Background(), byTitle["Node.js v17.0.0"], false); !res.OK {
		t.Errorf("clean stale version failed: %+v", res)
	}
	if _, err := os.Stat(filepath.Join(versions, "v22.18.0")); err != nil {
		t.Errorf("current version must be untouched: %v", err)
	}
}

func TestAIProviderDSH(t *testing.T) {
	home := t.TempDir()
	dsh := filepath.Join(home, ".dsh", "sessions")
	if err := os.MkdirAll(dsh, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dsh, "s1.json"), make([]byte, 128), 0o644); err != nil {
		t.Fatal(err)
	}
	// opencode with cache + config node_modules
	ocShare := filepath.Join(home, ".local", "share", "opencode", "log")
	if err := os.MkdirAll(ocShare, 0o755); err != nil {
		t.Fatal(err)
	}
	ocNM := filepath.Join(home, ".config", "opencode", "node_modules")
	if err := os.MkdirAll(ocNM, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(ocShare, "x.log"), make([]byte, 64), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(ocNM, "pkg"), make([]byte, 32), 0o644); err != nil {
		t.Fatal(err)
	}

	env := &Env{GOOS: "linux", Home: home, CacheDir: filepath.Join(home, ".cache"),
		AppData: filepath.Join(home, ".local", "share"), NoExternal: true}

	var got []*Target
	(&AIProvider{}).Scan(context.Background(), env, func(t *Target) { got = append(got, t) })

	found := map[string]bool{}
	for _, g := range got {
		for _, c := range g.Items {
			found[c.ID] = true
		}
		found[g.ID] = true
	}
	for _, want := range []string{"ai:dsh", "ai:dsh:sessions", "ai:dsh:all", "ai:opencode", "ai:opencode:log", "ai:opencode:node_modules", "ai:opencode:all"} {
		if !found[want] {
			t.Errorf("missing target %s in %+v", want, found)
		}
	}

	// the "delete all" entry must stay inside the home root
	for _, g := range got {
		if g.ID == "ai:dsh" {
			for _, c := range g.Items {
				if c.ID == "ai:dsh:all" && c.AllowedRoot != home {
					t.Errorf("allowedRoot should be home, got %s", c.AllowedRoot)
				}
			}
		}
	}
}
