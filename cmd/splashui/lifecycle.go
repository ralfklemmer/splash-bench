package main

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/ralfklemmer/splash-bench/internal/web"
)

// toolDirs hold the programs the checks call: system tools (sysctl,
// sw_vers, plutil) and Homebrew programs (splash). A Mac app started from
// Finder gets a short PATH without Homebrew; the system folders are added
// too, so the checks never depend on how the app was launched.
var toolDirs = []string{"/usr/bin", "/usr/sbin", "/bin", "/sbin", "/opt/homebrew/bin", "/usr/local/bin"}

// withToolDirs appends the directories missing from path.
func withToolDirs(path string) string {
	dirs := filepath.SplitList(path)
	for _, d := range toolDirs {
		if !slices.Contains(dirs, d) {
			dirs = append(dirs, d)
		}
	}
	return strings.Join(dirs, string(filepath.ListSeparator))
}

// insideAppBundle reports whether the executable runs from a .app bundle,
// i.e. was started by double-click rather than from a terminal.
func insideAppBundle(executable string) bool {
	return strings.Contains(executable, ".app/Contents/MacOS/")
}

// idleTimer decides when the app may quit because no page is open any more.
// It only counts while idle; any activity starts the wait anew.
type idleTimer struct {
	timeout time.Duration
	since   time.Time
}

func (t *idleTimer) expired(now time.Time, idle bool) bool {
	if !idle || t.timeout <= 0 {
		t.since = time.Time{}
		return false
	}
	if t.since.IsZero() {
		t.since = now
	}
	return now.Sub(t.since) >= t.timeout
}

// listenLoopback opens addr, which must be a loopback address.
func listenLoopback(addr string) (net.Listener, error) {
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		return nil, fmt.Errorf("parse -addr %q: %w", addr, err)
	}
	if ip := net.ParseIP(host); host != "localhost" && (ip == nil || !ip.IsLoopback()) {
		return nil, fmt.Errorf("-addr %q: die App lauscht nur auf localhost", addr)
	}
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		return nil, fmt.Errorf("listen on %s: %w", addr, err)
	}
	return listener, nil
}

// anyFreePort listens on a free port of the same host as addr.
func anyFreePort(addr string) (net.Listener, error) {
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		return nil, fmt.Errorf("parse -addr %q: %w", addr, err)
	}
	return listenLoopback(net.JoinHostPort(host, "0"))
}

// doer sends HTTP requests; *http.Client satisfies it.
type doer interface {
	Do(req *http.Request) (*http.Response, error)
}

// runningInstance reports whether Splash Bench already answers on addr.
func runningInstance(ctx context.Context, client doer, addr string) bool {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://"+addr+"/api/view", nil)
	if err != nil {
		return false
	}
	resp, err := client.Do(req)
	if err != nil {
		return false
	}
	resp.Body.Close()
	return resp.Header.Get(web.AppHeader) == "1"
}
