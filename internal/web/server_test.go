package web

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/ralfklemmer/splash-bench/internal/compare"
	"github.com/ralfklemmer/splash-bench/internal/readiness"
)

type fakeSession struct {
	startErr    error
	previousErr error
	started     []compare.Request
	cancelled   int
	shown       *compare.Result
	changes     chan struct{}
	view        compare.View
}

func newFakeSession() *fakeSession {
	return &fakeSession{changes: make(chan struct{}, 1), view: compare.View{Phase: compare.PhaseIdle}}
}

func (f *fakeSession) View() compare.View { return f.view }
func (f *fakeSession) Start(req compare.Request) error {
	f.started = append(f.started, req)
	return f.startErr
}
func (f *fakeSession) Cancel()             { f.cancelled++ }
func (f *fakeSession) Reset()              {}
func (f *fakeSession) ShowPrevious() error { return f.previousErr }
func (f *fakeSession) Shown() (compare.Result, bool) {
	if f.shown == nil {
		return compare.Result{}, false
	}
	return *f.shown, true
}
func (f *fakeSession) Subscribe() (<-chan struct{}, func()) { return f.changes, func() {} }

type fixedStatus readiness.Status

func (s fixedStatus) Status(context.Context) readiness.Status { return readiness.Status(s) }

func newTestServer(session Session) *Server {
	return NewServer(session, fixedStatus{Ready: true, Connected: true, Models: []readiness.ModelChoice{{ID: "m", Name: "M", Ready: true, Fits: true}}})
}

func do(h http.Handler, method, target, host, origin, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, target, strings.NewReader(body))
	req.Host = host
	if origin != "" {
		req.Header.Set("Origin", origin)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func TestGuard(t *testing.T) {
	tests := []struct {
		name   string
		method string
		host   string
		origin string
		want   int
	}{
		{"page on localhost", http.MethodGet, "localhost:8765", "", http.StatusOK},
		{"page on 127.0.0.1", http.MethodGet, "127.0.0.1:8765", "", http.StatusOK},
		{"page on ::1", http.MethodGet, "[::1]:8765", "", http.StatusOK},
		{"rebound DNS name", http.MethodGet, "evil.example:8765", "", http.StatusForbidden},
		{"start from the app", http.MethodPost, "127.0.0.1:8765", "http://127.0.0.1:8765", http.StatusAccepted},
		{"start without origin", http.MethodPost, "127.0.0.1:8765", "", http.StatusForbidden},
		{"start from another site", http.MethodPost, "127.0.0.1:8765", "https://evil.example", http.StatusForbidden},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			target, body := "/", ""
			if tt.method == http.MethodPost {
				target, body = "/api/start", `{"models":["m"],"prompts":["code"],"repeats":1}`
			}
			rec := do(newTestServer(newFakeSession()), tt.method, target, tt.host, tt.origin, body)
			if rec.Code != tt.want {
				t.Errorf("status = %d, want %d (%s)", rec.Code, tt.want, rec.Body.String())
			}
		})
	}
}

func TestPageIsEmbedded(t *testing.T) {
	rec := do(newTestServer(newFakeSession()), http.MethodGet, "/", "localhost:1", "", "")
	if !strings.Contains(rec.Body.String(), "Vergleich starten") {
		t.Errorf("index page missing, got %q", rec.Body.String()[:min(200, rec.Body.Len())])
	}
	if csp := rec.Header().Get("Content-Security-Policy"); !strings.Contains(csp, "default-src 'self'") {
		t.Errorf("CSP = %q", csp)
	}
}

func TestStartMapsErrors(t *testing.T) {
	tests := []struct {
		name string
		err  error
		body string
		want int
	}{
		{"accepted", nil, `{"models":["m"],"prompts":["code"],"repeats":2}`, http.StatusAccepted},
		{"invalid", fmt.Errorf("%w: kein Modell gewählt", compare.ErrInvalid), `{}`, http.StatusBadRequest},
		{"busy", compare.ErrBusy, `{}`, http.StatusConflict},
		{"broken JSON", nil, `{`, http.StatusBadRequest},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			session := newFakeSession()
			session.startErr = tt.err
			rec := do(newTestServer(session), http.MethodPost, "/api/start", "localhost:1", "http://localhost:1", tt.body)
			if rec.Code != tt.want {
				t.Fatalf("status = %d, want %d", rec.Code, tt.want)
			}
			if tt.want == http.StatusAccepted && (len(session.started) != 1 || session.started[0].Repeats != 2) {
				t.Errorf("started = %+v", session.started)
			}
			if tt.err != nil && !strings.Contains(rec.Body.String(), tt.err.Error()) {
				t.Errorf("body = %s, want the German message", rec.Body.String())
			}
		})
	}
}

func TestPreviousAndResult(t *testing.T) {
	session := newFakeSession()
	session.previousErr = compare.ErrNoPrevious
	srv := newTestServer(session)

	if rec := do(srv, http.MethodPost, "/api/previous", "localhost:1", "http://localhost:1", ""); rec.Code != http.StatusNotFound {
		t.Errorf("previous without result: status %d, want 404", rec.Code)
	}
	if rec := do(srv, http.MethodGet, "/api/result", "localhost:1", "", ""); rec.Code != http.StatusNotFound {
		t.Errorf("result without result: status %d, want 404", rec.Code)
	}

	session.shown = &compare.Result{MeasuredAt: time.Date(2026, 10, 17, 12, 30, 0, 0, time.Local), Repeats: 3}
	rec := do(srv, http.MethodGet, "/api/result", "localhost:1", "", "")
	if rec.Code != http.StatusOK || !strings.Contains(rec.Header().Get("Content-Disposition"), "splash-bench-2026-10-17-1230.json") {
		t.Errorf("result download: %d %q", rec.Code, rec.Header().Get("Content-Disposition"))
	}
	var r compare.Result
	if err := json.Unmarshal(rec.Body.Bytes(), &r); err != nil || r.Repeats != 3 {
		t.Errorf("result body = %s (%v)", rec.Body.String(), err)
	}
}

func TestCancel(t *testing.T) {
	session := newFakeSession()
	rec := do(newTestServer(session), http.MethodPost, "/api/cancel", "localhost:1", "http://localhost:1", "")
	if rec.Code != http.StatusAccepted || session.cancelled != 1 {
		t.Errorf("cancel: status %d, cancelled %d", rec.Code, session.cancelled)
	}
}

func TestEventsStreamViewOnChange(t *testing.T) {
	session := newFakeSession()
	srv := newTestServer(session)
	srv.minGap = 0
	ts := httptest.NewServer(srv)
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/api/events")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if ct := resp.Header.Get("Content-Type"); ct != "text/event-stream" {
		t.Fatalf("Content-Type = %q", ct)
	}
	events := bufio.NewScanner(resp.Body)
	next := func() compare.View {
		t.Helper()
		for events.Scan() {
			if data, ok := strings.CutPrefix(events.Text(), "data: "); ok {
				var v compare.View
				if err := json.Unmarshal([]byte(data), &v); err != nil {
					t.Fatalf("decode event: %v", err)
				}
				return v
			}
		}
		t.Fatalf("stream ended: %v", events.Err())
		return compare.View{}
	}

	if v := next(); v.Phase != compare.PhaseIdle {
		t.Errorf("first event phase = %s, want idle", v.Phase)
	}
	if n := srv.Clients(); n != 1 {
		t.Errorf("clients = %d, want 1 while the stream is open", n)
	}
	session.view = compare.View{Phase: compare.PhaseRunning, Done: 2, Total: 6}
	session.changes <- struct{}{}
	if v := next(); v.Phase != compare.PhaseRunning || v.Done != 2 {
		t.Errorf("second event = %+v", v)
	}
}

func TestStatus(t *testing.T) {
	rec := do(newTestServer(newFakeSession()), http.MethodGet, "/api/status", "localhost:1", "", "")
	var s readiness.Status
	if err := json.Unmarshal(rec.Body.Bytes(), &s); err != nil || !s.Ready || len(s.Models) != 1 {
		t.Errorf("status = %s (%v)", rec.Body.String(), err)
	}
}

func TestQuit(t *testing.T) {
	srv := newTestServer(newFakeSession())
	for range 2 { // a second click must not panic
		if rec := do(srv, http.MethodPost, "/api/quit", "localhost:1", "http://localhost:1", ""); rec.Code != http.StatusAccepted {
			t.Fatalf("quit: status %d", rec.Code)
		}
	}
	select {
	case <-srv.Quit():
	default:
		t.Fatal("Quit channel not closed")
	}
}

func TestResponsesCarryAppHeader(t *testing.T) {
	rec := do(newTestServer(newFakeSession()), http.MethodGet, "/api/status", "localhost:1", "", "")
	if rec.Header().Get(AppHeader) != "1" {
		t.Errorf("header %s missing", AppHeader)
	}
}
