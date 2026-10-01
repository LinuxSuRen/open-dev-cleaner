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

// TestCleanOllamaTempServer reproduces the reported bug: scanning lists
// models via a temporary `ollama serve`, the server stops, and a later
// `ollama rm` would fail with "could not connect". The cleaner must boot
// the server again for the command, then stop it.
func TestCleanOllamaTempServer(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell script based test, unix only")
	}
	home := t.TempDir()
	bin := t.TempDir()
	marker := filepath.Join(bin, "server-up")
	model := filepath.Join(home, "model-gone-check")

	// fake `ollama`:
	//   serve -> touch marker, sleep (simulates the daemon)
	//   list  -> works only while marker exists
	//   rm    -> deletes the model marker file, only while server up
	script := "#!/bin/sh\n" +
		"case \"$1\" in\n" +
		"  serve) touch " + marker + "; sleep 60 ;;\n" +
		"  list) if [ -f " + marker + " ]; then printf 'NAME ID SIZE MODIFIED\\ndemo:7b abc123 4.7GB 2 days ago\\n'; else echo 'cannot connect to server' >&2; exit 1; fi ;;\n" +
		"  rm) if [ -f " + marker + " ]; then rm -f " + model + "; exit 0; else echo 'Error: could not connect to ollama app, is it running?' >&2; exit 1; fi ;;\n" +
		"esac\n"
	if err := os.WriteFile(filepath.Join(bin, "ollama"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(home, ".ollama", "models"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(model, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("OLLAMA_HOST", "")

	env := &Env{GOOS: runtime.GOOS, Home: home,
		CacheDir: filepath.Join(home, ".cache"), AppData: filepath.Join(home, ".local", "share")}

	// 1) scan: temp server starts, model listed, server stopped afterwards.
	var got []*Target
	(&OllamaProvider{}).Scan(context.Background(), env, func(t *Target) { got = append(got, t) })
	if len(got) != 1 || len(got[0].Items) != 1 {
		t.Fatalf("scan did not list models: %+v", got)
	}
	// simulate "server stopped again" (serve only touches; remove marker)
	if err := os.Remove(marker); err != nil {
		t.Fatal(err)
	}

	// 2) clean the model without a running server.
	var log strings.Builder
	cleaner := NewCleaner(env, &log)
	res := cleaner.Clean(context.Background(), got[0].Items[0], false)
	if !res.OK {
		t.Fatalf("clean failed: %+v\nlog:\n%s", res, log.String())
	}
	if _, err := os.Stat(model); err == nil {
		t.Fatal("fake model file still exists, rm did not run")
	}
	if !strings.Contains(log.String(), "临时启动") || !strings.Contains(log.String(), "已关闭") {
		t.Errorf("temp server lifecycle not logged:\n%s", log.String())
	}
	// 3) the spawned server must be stopped: marker remains from serve,
	// but no `sleep 60` of ours may outlive the test — verified via the
	// stop path being invoked (log line) and process reaping in stop().
	time.Sleep(100 * time.Millisecond)
}

func TestStripANSI(t *testing.T) {
	in := "\x1b[?25l\x1b[1G\x1b[Kdeleted 'demo:7b'\r\x1b[2K"
	if got := stripANSI(in); got != "deleted 'demo:7b'" {
		t.Fatalf("stripANSI = %q", got)
	}
	if got := stripANSI("plain line"); got != "plain line" {
		t.Fatalf("plain text altered: %q", got)
	}
}
