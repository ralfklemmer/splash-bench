// Package web serves the Splash Bench app on localhost: the page, a small
// JSON API and a stream of live updates (Server-Sent Events).
package web

import (
	"context"
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"net"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/ralfklemmer/splash-bench/internal/compare"
	"github.com/ralfklemmer/splash-bench/internal/readiness"
)

//go:embed static
var staticFiles embed.FS

// Session is the comparison the app controls.
type Session interface {
	View() compare.View
	Start(req compare.Request) error
	Cancel()
	Reset()
	ShowPrevious() error
	Shown() (compare.Result, bool)
	Subscribe() (<-chan struct{}, func())
}

// StatusSource checks the requirements and lists the models to choose from.
type StatusSource interface {
	Status(ctx context.Context) readiness.Status
}

// AppHeader marks responses of Splash Bench, so a second start can find a
// running instance instead of opening another one.
const AppHeader = "X-Splash-Bench"

// Server is the app's HTTP handler.
type Server struct {
	session Session
	status  StatusSource
	// minGap throttles live updates per client.
	minGap    time.Duration
	keepAlive time.Duration
	mux       *http.ServeMux
	clients   atomic.Int64
	quit      chan struct{}
	quitOnce  sync.Once
}

func NewServer(session Session, status StatusSource) *Server {
	s := &Server{session: session, status: status, minGap: 100 * time.Millisecond, keepAlive: 15 * time.Second, mux: http.NewServeMux(), quit: make(chan struct{})}
	static, err := fs.Sub(staticFiles, "static")
	if err != nil {
		panic(err) // the embedded directory is part of the binary
	}
	s.mux.Handle("GET /", http.FileServerFS(static))
	s.mux.HandleFunc("GET /api/status", s.handleStatus)
	s.mux.HandleFunc("GET /api/view", s.handleView)
	s.mux.HandleFunc("GET /api/events", s.handleEvents)
	s.mux.HandleFunc("GET /api/result", s.handleResult)
	s.mux.HandleFunc("POST /api/start", s.handleStart)
	s.mux.HandleFunc("POST /api/cancel", s.handleCancel)
	s.mux.HandleFunc("POST /api/reset", s.handleReset)
	s.mux.HandleFunc("POST /api/previous", s.handlePrevious)
	s.mux.HandleFunc("POST /api/quit", s.handleQuit)
	return s
}

// Quit is closed when the user ends the app from the page.
func (s *Server) Quit() <-chan struct{} {
	return s.quit
}

// Clients is the number of pages currently following the live updates.
func (s *Server) Clients() int {
	return int(s.clients.Load())
}

// ServeHTTP only answers requests addressed to the loopback interface, and
// changes only from the app's own page, so a web page in another tab cannot
// switch engines (DNS rebinding, cross-site requests).
func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if !isLoopbackHost(r.Host) {
		http.Error(w, "nur über localhost erreichbar", http.StatusForbidden)
		return
	}
	if r.Method != http.MethodGet && r.Method != http.MethodHead && r.Header.Get("Origin") != "http://"+r.Host {
		http.Error(w, "Anfrage nicht von dieser App", http.StatusForbidden)
		return
	}
	w.Header().Set(AppHeader, "1")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Content-Security-Policy", "default-src 'self'; style-src 'self'; img-src 'self' data:; connect-src 'self'; frame-ancestors 'none'")
	s.mux.ServeHTTP(w, r)
}

func isLoopbackHost(hostport string) bool {
	host, _, err := net.SplitHostPort(hostport)
	if err != nil {
		host = hostport
	}
	if host == "localhost" {
		return true
	}
	ip := net.ParseIP(strings.Trim(host, "[]"))
	return ip != nil && ip.IsLoopback()
}

func (s *Server) handleStatus(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, s.status.Status(r.Context()))
}

func (s *Server) handleView(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, s.session.View())
}

func (s *Server) handleStart(w http.ResponseWriter, r *http.Request) {
	var req compare.Request
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 64<<10)).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "Anfrage nicht lesbar")
		return
	}
	switch err := s.session.Start(req); {
	case errors.Is(err, compare.ErrInvalid):
		writeError(w, http.StatusBadRequest, err.Error())
	case errors.Is(err, compare.ErrBusy):
		writeError(w, http.StatusConflict, err.Error())
	case err != nil:
		writeError(w, http.StatusInternalServerError, err.Error())
	default:
		writeJSON(w, http.StatusAccepted, s.session.View())
	}
}

func (s *Server) handleQuit(w http.ResponseWriter, _ *http.Request) {
	s.quitOnce.Do(func() { close(s.quit) })
	w.WriteHeader(http.StatusAccepted)
}

func (s *Server) handleCancel(w http.ResponseWriter, _ *http.Request) {
	s.session.Cancel()
	w.WriteHeader(http.StatusAccepted)
}

func (s *Server) handleReset(w http.ResponseWriter, _ *http.Request) {
	s.session.Reset()
	writeJSON(w, http.StatusOK, s.session.View())
}

func (s *Server) handlePrevious(w http.ResponseWriter, _ *http.Request) {
	switch err := s.session.ShowPrevious(); {
	case errors.Is(err, compare.ErrNoPrevious):
		writeError(w, http.StatusNotFound, err.Error())
	case errors.Is(err, compare.ErrBusy):
		writeError(w, http.StatusConflict, err.Error())
	case err != nil:
		writeError(w, http.StatusInternalServerError, err.Error())
	default:
		writeJSON(w, http.StatusOK, s.session.View())
	}
}

// handleResult offers the shown result as a download.
func (s *Server) handleResult(w http.ResponseWriter, _ *http.Request) {
	result, ok := s.session.Shown()
	if !ok {
		writeError(w, http.StatusNotFound, "kein Ergebnis zum Speichern")
		return
	}
	name := "splash-bench-" + result.MeasuredAt.Local().Format("2006-01-02-1504") + ".json"
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", name))
	writeJSON(w, http.StatusOK, result)
}

// handleEvents streams the view after every change, at most every minGap.
func (s *Server) handleEvents(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		writeError(w, http.StatusInternalServerError, "Streaming nicht möglich")
		return
	}
	changes, unsubscribe := s.session.Subscribe()
	defer unsubscribe()
	s.clients.Add(1)
	defer s.clients.Add(-1)

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-store")
	keepAlive := time.NewTicker(s.keepAlive)
	defer keepAlive.Stop()

	ctx := r.Context()
	for {
		if err := writeEvent(w, s.session.View()); err != nil {
			return
		}
		flusher.Flush()
		select {
		case <-ctx.Done():
			return
		case <-time.After(s.minGap):
		}
		if !waitForChange(ctx, w, flusher, changes, keepAlive.C) {
			return
		}
	}
}

// waitForChange blocks until the session changes, sending comments to keep
// the connection open; it reports false when the client is gone.
func waitForChange(ctx context.Context, w http.ResponseWriter, flusher http.Flusher, changes <-chan struct{}, keepAlive <-chan time.Time) bool {
	for {
		select {
		case <-ctx.Done():
			return false
		case <-changes:
			return true
		case <-keepAlive:
			if _, err := fmt.Fprint(w, ": keep-alive\n\n"); err != nil {
				return false
			}
			flusher.Flush()
		}
	}
}

func writeEvent(w http.ResponseWriter, v compare.View) error {
	data, err := json.Marshal(v)
	if err != nil {
		return fmt.Errorf("encode view: %w", err)
	}
	_, err = fmt.Fprintf(w, "data: %s\n\n", data)
	return err
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, code int, message string) {
	writeJSON(w, code, map[string]string{"error": message})
}
