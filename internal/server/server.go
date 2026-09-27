// Package server exposes the scan/clean engine over HTTP with SSE
// streaming, plus the embedded Vue frontend.
package server

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/LinuxSuRen/open-dev-cleaner/internal/scan"
)

// Version information, injected via -ldflags at build time.
var (
	Version   = "dev"
	Commit    = "none"
	BuildDate = "unknown"
)

// Server holds shared state between HTTP handlers.
type Server struct {
	engine *scan.Engine
	env    *scan.Env
	web    fs.FS

	mu       sync.Mutex
	lastScan []*scan.Target
	// version of lastScan, bumped after each completed scan and after
	// cleaning so the UI knows when to refresh.
	scanEpoch int
}

// New builds a server serving the web filesystem (already rooted at its
// content directory).
func New(env *scan.Env, web fs.FS) *Server {
	return &Server{engine: scan.NewEngine(env), env: env, web: web}
}

// Handler returns the routed HTTP handler.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /api/version", s.handleVersion)
	mux.HandleFunc("GET /api/disks", s.handleDisks)
	mux.HandleFunc("GET /api/last", s.handleLast)
	mux.HandleFunc("GET /api/scan", s.handleScan)
	mux.HandleFunc("POST /api/clean", s.handleClean)
	mux.HandleFunc("GET /{$}", s.handleIndex)
	mux.HandleFunc("GET /static/", s.handleStatic)

	return logRequests(mux)
}

// ListenAndServe starts the HTTP server.
func (s *Server) ListenAndServe(ctx context.Context, addr string) error {
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return err
	}
	srv := &http.Server{Handler: s.Handler(), ReadHeaderTimeout: 10 * time.Second}
	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		_ = srv.Shutdown(shutdownCtx)
	}()
	fmt.Printf("Open Dev Cleaner %s 已启动: http://%s\n", Version, displayAddr(addr))
	return srv.Serve(ln)
}

func displayAddr(addr string) string {
	if strings.HasPrefix(addr, ":") {
		return "127.0.0.1" + addr
	}
	if strings.Contains(addr, "0.0.0.0") {
		if ip, err := localIP(); err == nil {
			return strings.Replace(addr, "0.0.0.0", ip, 1)
		}
	}
	return addr
}

func localIP() (string, error) {
	conn, err := net.Dial("udp", "8.8.8.8:80")
	if err != nil {
		return "", err
	}
	defer conn.Close()
	return conn.LocalAddr().(*net.UDPAddr).IP.String(), nil
}

// --- SSE helpers ---

type sseWriter struct {
	w       http.ResponseWriter
	flusher http.Flusher
}

func startSSE(w http.ResponseWriter) (*sseWriter, bool) {
	f, ok := w.(http.Flusher)
	if !ok {
		return nil, false
	}
	w.Header().Set("Content-Type", "text/event-stream; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)
	f.Flush()
	return &sseWriter{w: w, flusher: f}, true
}

func (s *sseWriter) send(v any) {
	b, _ := json.Marshal(v)
	fmt.Fprintf(s.w, "data: %s\n\n", b)
	s.flusher.Flush()
}

// --- handlers ---

func (s *Server) handleVersion(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, map[string]any{
		"version": Version, "commit": Commit, "buildDate": BuildDate,
		"goos": s.env.GOOS, "home": s.env.Home,
	})
}

// handleDisks reports per-volume disk usage (platform specific:
// drive letters on Windows, mount points on Linux, APFS volumes on macOS).
func (s *Server) handleDisks(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, map[string]any{"disks": scan.ListVolumes()})
}

func (s *Server) handleLast(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	targets := s.lastScan
	s.mu.Unlock()
	writeJSON(w, map[string]any{"targets": targets, "summary": scan.Summarize(targets)})
}

// handleScan runs a fresh scan. Default mode is SSE streaming; pass
// ?format=json to wait for the full result as a single JSON document
// (handy for CLI/tests).
func (s *Server) handleScan(w http.ResponseWriter, r *http.Request) {
	if r.URL.Query().Get("format") != "json" {
		if sw, ok := startSSE(w); ok {
			var collected []*scan.Target
			collected = s.engine.Run(r.Context(), func(ev scan.Event) {
				if ev.Type == "target" && ev.Target != nil {
					collected = append(collected, ev.Target)
				}
				sw.send(ev)
			})
			s.mu.Lock()
			s.lastScan = collected
			s.scanEpoch++
			s.mu.Unlock()
			return
		}
	}

	var collected []*scan.Target
	collected = s.engine.Run(r.Context(), func(ev scan.Event) {
		if ev.Type == "target" && ev.Target != nil {
			collected = append(collected, ev.Target)
		}
	})
	s.mu.Lock()
	s.lastScan = collected
	s.scanEpoch++
	s.mu.Unlock()
	writeJSON(w, map[string]any{"targets": collected, "summary": scan.Summarize(collected)})
}

type cleanRequest struct {
	IDs           []string `json:"ids"`
	DangerConfirm bool     `json:"dangerConfirm"`
	DryRun        bool     `json:"dryRun"`
}

func (s *Server) handleClean(w http.ResponseWriter, r *http.Request) {
	var req cleanRequest
	body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	if err != nil || json.Unmarshal(body, &req) != nil || len(req.IDs) == 0 {
		http.Error(w, `{"error":"请求体应为 {"ids":["..."]}"}`, http.StatusBadRequest)
		return
	}

	s.mu.Lock()
	last := s.lastScan
	s.mu.Unlock()

	// Resolve targets against the last scan result.
	var targets []*scan.Target
	var unknown []string
	hasDangerous := false
	for _, id := range req.IDs {
		t := scan.FindTarget(last, id)
		if t == nil {
			unknown = append(unknown, id)
			continue
		}
		targets = append(targets, t)
		if scan.RiskOrder(t.Risk) >= scan.RiskOrder(scan.RiskDangerous) {
			hasDangerous = true
		}
	}
	if hasDangerous && !req.DangerConfirm {
		http.Error(w, `{"error":"包含危险项,需要 dangerConfirm 确认"}`, http.StatusBadRequest)
		return
	}

	sw, ok := startSSE(w)
	if !ok {
		// Non-streaming fallback: run synchronously.
		results := s.runClean(r.Context(), targets, req.DryRun, io.Discard)
		writeJSON(w, map[string]any{"results": results})
		return
	}

	if len(unknown) > 0 {
		sw.send(scan.Event{Type: "log", Message: "以下目标未在最近扫描中找到,已忽略: " + strings.Join(unknown, ", ") + "(请重新扫描)"})
	}

	// Stream cleaner output through a pipe.
	pr, pw := io.Pipe()
	done := make(chan []scan.CleanResult, 1)
	go func() {
		results := s.runClean(r.Context(), targets, req.DryRun, pw)
		_ = pw.Close()
		done <- results
	}()
	go func() {
		buf := make([]byte, 4096)
		var line []byte
		for {
			n, err := pr.Read(buf)
			if n > 0 {
				line = append(line, buf[:n]...)
				for {
					i := indexByte(line, '\n')
					if i < 0 {
						break
					}
					msg := strings.TrimSpace(string(line[:i]))
					line = line[i+1:]
					if msg != "" {
						sw.send(scan.Event{Type: "log", Message: msg})
					}
				}
			}
			if err != nil {
				break
			}
		}
	}()

	results := <-done
	var freed int64
	fails := 0
	for _, res := range results {
		if res.OK {
			freed += res.Freed
		} else {
			fails++
		}
	}
	sw.send(scan.Event{Type: "done", Message: fmt.Sprintf("清理完成: %d 项,释放约 %s,%d 项失败", len(results), scan.FormatSize(freed), fails), Summary: &scan.Summary{TotalSize: freed}})
}

func indexByte(b []byte, c byte) int {
	for i := range b {
		if b[i] == c {
			return i
		}
	}
	return -1
}

func (s *Server) runClean(ctx context.Context, targets []*scan.Target, dry bool, out io.Writer) []scan.CleanResult {
	cleaner := scan.NewCleaner(s.env, out)
	var results []scan.CleanResult
	for _, t := range targets {
		if ctx.Err() != nil {
			break
		}
		res := cleaner.Clean(ctx, t, dry)
		results = append(results, res)
	}
	return results
}

// --- static files ---

var contentTypeByExt = map[string]string{
	".html": "text/html; charset=utf-8",
	".js":   "text/javascript; charset=utf-8",
	".css":  "text/css; charset=utf-8",
	".svg":  "image/svg+xml",
	".png":  "image/png",
	".ico":  "image/x-icon",
	".woff2": "font/woff2",
}

func (s *Server) handleIndex(w http.ResponseWriter, r *http.Request) {
	data, err := fs.ReadFile(s.web, "index.html")
	if err != nil {
		http.Error(w, "index.html missing", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache")
	_, _ = w.Write(data)
}

func (s *Server) handleStatic(w http.ResponseWriter, r *http.Request) {
	name := strings.TrimPrefix(r.URL.Path, "/static/")
	if name == "" || strings.Contains(name, "..") {
		http.NotFound(w, r)
		return
	}
	data, err := fs.ReadFile(s.web, name)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	ext := name[strings.LastIndexByte(name, '.'):]
	if ct, ok := contentTypeByExt[ext]; ok {
		w.Header().Set("Content-Type", ct)
	}
	w.Header().Set("Cache-Control", "no-cache")
	_, _ = w.Write(data)
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	_ = json.NewEncoder(w).Encode(v)
}

func logRequests(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		next.ServeHTTP(w, r)
	})
}
