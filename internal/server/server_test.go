package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/LinuxSuRen/open-dev-cleaner/internal/scan"
)

func testEnv(t *testing.T) *scan.Env {
	t.Helper()
	return &scan.Env{
		GOOS: "linux", Home: t.TempDir(),
		CacheDir: t.TempDir(), AppData: t.TempDir(),
		NoExternal: true,
	}
}

func newTestServer(t *testing.T, last []*scan.Target) *Server {
	t.Helper()
	web := fstest.MapFS{
		"index.html": &fstest.MapFile{Data: []byte("<html>odc</html>")},
		"css/x.css":  &fstest.MapFile{Data: []byte("body{}")},
	}
	s := New(testEnv(t), web)
	s.lastScan = last
	return s
}

func TestCleanRequiresDangerConfirm(t *testing.T) {
	last := []*scan.Target{{
		ID: "docker:volumes", Tool: "docker", ToolTitle: "Docker",
		Title: "数据卷", Risk: scan.RiskDangerous,
		Method: scan.MethodCommand, Cmd: []string{"true"}, Available: true,
	}}
	s := newTestServer(t, last)

	// without confirm -> rejected
	req := httptest.NewRequest(http.MethodPost, "/api/clean",
		strings.NewReader(`{"ids":["docker:volumes"]}`))
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("dangerous clean without confirm: got %d, want 400", rec.Code)
	}

	// with confirm + dry-run -> streamed
	req = httptest.NewRequest(http.MethodPost, "/api/clean",
		strings.NewReader(`{"ids":["docker:volumes"],"dangerConfirm":true,"dryRun":true}`))
	rec = httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("dangerous dry-run: got %d, want 200 (body %s)", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "dry-run") {
		t.Fatalf("expected dry-run output, got: %s", rec.Body.String())
	}
}

func TestCleanUnknownID(t *testing.T) {
	s := newTestServer(t, nil)
	req := httptest.NewRequest(http.MethodPost, "/api/clean",
		strings.NewReader(`{"ids":["nope"]}`))
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("got %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "未在最近扫描中找到") {
		t.Fatalf("expected unknown-id warning, got: %s", rec.Body.String())
	}
}

func TestCleanGroupExpandsChildren(t *testing.T) {
	tmp := t.TempDir()
	dir := tmp + "/inner"
	if err := mkdirAll(dir); err != nil {
		t.Fatal(err)
	}
	last := []*scan.Target{{
		ID: "grp", Tool: "t", ToolTitle: "T", Title: "group",
		Risk: scan.RiskSafe, Method: scan.MethodGroup, Available: true,
		Items: []*scan.Target{{
			ID: "child", Tool: "t", ToolTitle: "T", Title: "child",
			Path: dir, AllowedRoot: tmp, Risk: scan.RiskSafe,
			Method: scan.MethodDir, Available: true,
		}},
	}}
	s := newTestServer(t, last)
	// point cleaner env at the same temp tree
	s.env.Home = tmp

	req := httptest.NewRequest(http.MethodPost, "/api/clean",
		strings.NewReader(`{"ids":["grp"]}`))
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	body := rec.Body.String()
	if !strings.Contains(body, "已删除目录") {
		t.Fatalf("group should clean child dir, got: %s", body)
	}
	if exists(dir) {
		t.Fatal("child dir should be removed")
	}
}

func TestScanJSONEndpoint(t *testing.T) {
	s := newTestServer(t, nil)
	req := httptest.NewRequest(http.MethodGet, "/api/scan?format=json", nil)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("scan json: %d", rec.Code)
	}
	var out struct {
		Summary *scan.Summary `json:"summary"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("invalid json: %v", err)
	}
	if out.Summary == nil {
		t.Fatal("summary missing")
	}
}

func TestStaticServing(t *testing.T) {
	s := newTestServer(t, nil)
	h := s.Handler()

	for path, want := range map[string]int{"/": 200, "/static/css/x.css": 200, "/nope": 404} {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		if rec.Code != want {
			t.Fatalf("GET %s: got %d want %d", path, rec.Code, want)
		}
	}

	// Path traversal is neutralized by the mux redirect and by the handler.
	req := httptest.NewRequest(http.MethodGet, "/static/../index.html", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code == http.StatusOK && strings.Contains(rec.Body.String(), "odc") {
		t.Fatalf("traversal unexpectedly served file content: %d %s", rec.Code, rec.Body.String())
	}
}

func mkdirAll(p string) error { return os.MkdirAll(p, 0o755) }

func exists(p string) bool {
	_, err := os.Stat(p)
	return err == nil
}
