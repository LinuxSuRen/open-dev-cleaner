package scan

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

// TestOllamaTempServer simulates a machine where the ollama binary exists
// but the server is not running: `ollama serve` boots a fake "server"
// (creates a marker file), after which `ollama list` starts working. The
// provider must start the server, scan the models and stop the process
// it spawned.
func TestOllamaTempServer(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell script based test, unix only")
	}
	home := t.TempDir()
	bin := t.TempDir()
	marker := filepath.Join(bin, "server-up")

	// fake `ollama`: serve -> touch marker & sleep; list -> works only
	// once the marker exists.
	script := "#!/bin/sh\n" +
		"case \"$1\" in\n" +
		"  serve) touch " + marker + "; sleep 60 ;;\n" +
		"  list) if [ -f " + marker + " ]; then printf 'NAME ID SIZE MODIFIED\\ndemo:7b abc123 4.7GB 2 days ago\\n'; else echo 'cannot connect to server' >&2; exit 1; fi ;;\n" +
		"esac\n"
	if err := os.WriteFile(filepath.Join(bin, "ollama"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}

	// The models dir must exist for the provider to engage.
	if err := os.MkdirAll(filepath.Join(home, ".ollama", "models"), 0o755); err != nil {
		t.Fatal(err)
	}

	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("OLLAMA_HOST", "") // ensure auto-start is allowed

	env := &Env{GOOS: runtime.GOOS, Home: home,
		CacheDir: filepath.Join(home, ".cache"), AppData: filepath.Join(home, ".local", "share")}

	var got []*Target
	done := make(chan struct{})
	go func() {
		defer close(done)
		(&OllamaProvider{}).Scan(context.Background(), env, func(t *Target) { got = append(got, t) })
	}()
	select {
	case <-done:
	case <-time.After(40 * time.Second):
		t.Fatal("scan with temp server did not finish")
	}

	if len(got) != 1 || got[0].ID != "ollama:models" {
		t.Fatalf("expected models group, got %+v", got)
	}
	g := got[0]
	if !strings.Contains(g.Note, "临时启动") {
		t.Errorf("temp-server note missing: %q", g.Note)
	}
	if len(g.Items) != 1 || g.Items[0].Title != "demo:7b" {
		t.Errorf("model child missing: %+v", g.Items)
	}

	// The spawned fake server must have run (marker exists) ...
	if _, err := os.Stat(marker); err != nil {
		t.Errorf("server never started (marker missing)")
	}
}
