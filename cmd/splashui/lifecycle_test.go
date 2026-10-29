package main

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/ralfklemmer/splash-bench/internal/web"
)

func TestWithToolDirs(t *testing.T) {
	tests := []struct {
		path string
		want string
	}{
		{"/usr/bin:/bin", "/usr/bin:/bin:/usr/sbin:/sbin:/opt/homebrew/bin:/usr/local/bin"},
		{"/opt/homebrew/bin:/usr/bin", "/opt/homebrew/bin:/usr/bin:/usr/sbin:/bin:/sbin:/usr/local/bin"},
		{"/usr/bin:/usr/sbin:/bin:/sbin:/opt/homebrew/bin:/usr/local/bin", "/usr/bin:/usr/sbin:/bin:/sbin:/opt/homebrew/bin:/usr/local/bin"},
		{"", "/usr/bin:/usr/sbin:/bin:/sbin:/opt/homebrew/bin:/usr/local/bin"},
	}
	for _, tt := range tests {
		if got := withToolDirs(tt.path); got != tt.want {
			t.Errorf("withToolDirs(%q) = %q, want %q", tt.path, got, tt.want)
		}
	}
}

func TestInsideAppBundle(t *testing.T) {
	if !insideAppBundle("/Applications/Splash Bench.app/Contents/MacOS/splashui") {
		t.Error("bundle path not recognised")
	}
	if insideAppBundle("/Users/ralf/splash-kit/bin/splashui") {
		t.Error("plain binary taken for a bundle")
	}
}

func TestIdleTimer(t *testing.T) {
	t0 := time.Date(2026, 10, 24, 20, 0, 0, 0, time.UTC)
	steps := []struct {
		after time.Duration
		idle  bool
		want  bool
	}{
		{0, true, false},
		{90 * time.Second, true, false},
		{100 * time.Second, false, false}, // a page reconnected: start over
		{110 * time.Second, true, false},
		{229 * time.Second, true, false},
		{230 * time.Second, true, true},
	}
	timer := idleTimer{timeout: 2 * time.Minute}
	for _, s := range steps {
		if got := timer.expired(t0.Add(s.after), s.idle); got != s.want {
			t.Errorf("after %v (idle %v): expired = %v, want %v", s.after, s.idle, got, s.want)
		}
	}
	never := idleTimer{}
	if never.expired(t0, true) || never.expired(t0.Add(time.Hour), true) {
		t.Error("timeout 0 must never expire")
	}
}

func TestRunningInstance(t *testing.T) {
	ours := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set(web.AppHeader, "1")
	}))
	defer ours.Close()
	other := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	defer other.Close()

	ctx := context.Background()
	if !runningInstance(ctx, http.DefaultClient, strings.TrimPrefix(ours.URL, "http://")) {
		t.Error("own instance not found")
	}
	if runningInstance(ctx, http.DefaultClient, strings.TrimPrefix(other.URL, "http://")) {
		t.Error("another program taken for Splash Bench")
	}
	if runningInstance(ctx, http.DefaultClient, "127.0.0.1:1") {
		t.Error("nothing listening, but instance found")
	}
}

func TestListenOnlyOnLoopback(t *testing.T) {
	if _, err := listenLoopback("0.0.0.0:0"); err == nil {
		t.Error("listen on all interfaces must be refused")
	}
	l, err := listenLoopback("127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer l.Close()
	taken := l.Addr().String()
	if _, err := listenLoopback(taken); err == nil {
		t.Error("a taken port must fail")
	}
	free, err := anyFreePort(taken)
	if err != nil {
		t.Fatalf("anyFreePort: %v", err)
	}
	defer free.Close()
	if free.Addr().String() == taken || !free.Addr().(*net.TCPAddr).IP.IsLoopback() {
		t.Errorf("free port = %s", free.Addr())
	}
}
