package scan

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func mkFile(t *testing.T, path string, size int) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, make([]byte, size), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestEditorJetBrainsPerProduct(t *testing.T) {
	home := t.TempDir()
	cache := filepath.Join(home, "Library", "Caches")
	env := &Env{GOOS: "darwin", Home: home, CacheDir: cache,
		AppData: filepath.Join(home, "Library", "Application Support"), NoExternal: true}

	mkFile(t, filepath.Join(cache, "JetBrains", "PyCharm2024.1", "index.bin"), 300)
	mkFile(t, filepath.Join(cache, "JetBrains", "IntelliJIdea2023.2", "index.bin"), 200)
	// stale product
	stale := filepath.Join(cache, "JetBrains", "GoLand2022.3")
	mkFile(t, filepath.Join(stale, "index.bin"), 100)
	touchOld(t, stale, 300)
	mkFile(t, filepath.Join(cache, "Google", "AndroidStudio2024.1", "caches.bin"), 150)

	var got []*Target
	(&EditorProvider{}).Scan(context.Background(), env, func(t *Target) { got = append(got, t) })

	var jb *Target
	asSeen := false
	for _, g := range got {
		if g.ID == "editor:jetbrains" {
			jb = g
		}
		if g.ID == "editor:android-studio:AndroidStudio2024.1" {
			asSeen = true
		}
	}
	if jb == nil {
		t.Fatalf("jetbrains group missing in %+v", ids(got))
	}
	if len(jb.Items) != 3 {
		t.Fatalf("want 3 products, got %d", len(jb.Items))
	}
	notes := map[string]string{}
	for _, c := range jb.Items {
		notes[c.Title] = c.Note
	}
	if notes["GoLand2022.3"] == "" {
		t.Errorf("stale product should carry hint: %+v", notes)
	}
	if notes["PyCharm2024.1"] != "" {
		t.Errorf("recent product should not carry hint: %+v", notes)
	}
	if !asSeen {
		t.Errorf("android studio target missing: %+v", ids(got))
	}
}

func TestEclipseTargets(t *testing.T) {
	home := t.TempDir()
	env := &Env{GOOS: "linux", Home: home, CacheDir: filepath.Join(home, ".cache"),
		AppData: filepath.Join(home, ".local", "share"), NoExternal: true}

	mkFile(t, filepath.Join(home, ".p2", "org.eclipse.equinox.p2.core", "cache", "blob"), 100)
	mkFile(t, filepath.Join(home, ".p2", "pool", "plugins", "org.eclipse.core"), 500)
	mkFile(t, filepath.Join(home, ".eclipse", "lock"), 10)

	var got []*Target
	(&EditorProvider{}).Scan(context.Background(), env, func(t *Target) { got = append(got, t) })

	found := map[string]*Target{}
	for _, g := range got {
		found[g.ID] = g
	}
	for _, want := range []string{"editor:eclipse-p2-cache", "editor:eclipse-p2-pool", "editor:eclipse-data"} {
		if found[want] == nil {
			t.Errorf("missing %s in %+v", want, ids(got))
		}
	}
	if found["editor:eclipse-p2-cache"] != nil && found["editor:eclipse-p2-cache"].Risk != RiskSafe {
		t.Errorf("p2 cache should be safe")
	}
	if found["editor:eclipse-p2-pool"] != nil && found["editor:eclipse-p2-pool"].Risk != RiskHigh {
		t.Errorf("p2 pool should be high risk")
	}
}

func TestDotNetProvider(t *testing.T) {
	home := t.TempDir()
	env := &Env{GOOS: "linux", Home: home, CacheDir: filepath.Join(home, ".cache"),
		AppData: filepath.Join(home, ".local", "share"), NoExternal: true}

	mkFile(t, filepath.Join(home, ".nuget", "packages", "newtonsoft.json", "13.0.3", "lib.dll"), 700)
	mkFile(t, filepath.Join(home, ".local", "share", "NuGet", "http-cache", "pkg.zip"), 200)

	var got []*Target
	(&DotNetProvider{}).Scan(context.Background(), env, func(t *Target) { got = append(got, t) })

	found := map[string]*Target{}
	for _, g := range got {
		found[g.ID] = g
	}
	if found["dotnet:global-packages"] == nil || found["dotnet:global-packages"].Size != 700 {
		t.Errorf("global packages target wrong: %+v", found["dotnet:global-packages"])
	}
	if found["dotnet:http-cache"] == nil || found["dotnet:http-cache"].Size != 200 {
		t.Errorf("http cache target wrong: %+v", found["dotnet:http-cache"])
	}
}

func ids(ts []*Target) []string {
	out := make([]string, 0, len(ts))
	for _, t := range ts {
		out = append(out, t.ID)
	}
	return out
}
