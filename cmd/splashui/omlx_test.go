package main

import (
	"errors"
	"fmt"
	"testing"
	"testing/fstest"

	"github.com/ralfklemmer/splash-bench/internal/omlx"
	"github.com/ralfklemmer/splash-bench/internal/readiness"
)

func settingsFS(json string) fstest.MapFS {
	return fstest.MapFS{"Users/ralf/.omlx/settings.json": {Data: []byte(json)}}
}

func TestBaseURLFromSettings(t *testing.T) {
	tests := []struct {
		name  string
		files fstest.MapFS
		want  string
		ok    bool
	}{
		{"all interfaces", settingsFS(`{"server":{"host":"0.0.0.0","port":8000}}`), "http://localhost:8000", true},
		{"other port", settingsFS(`{"server":{"host":"127.0.0.1","port":9123}}`), "http://127.0.0.1:9123", true},
		{"IPv6 any", settingsFS(`{"server":{"host":"::","port":8001}}`), "http://localhost:8001", true},
		{"no port", settingsFS(`{"server":{}}`), fallbackOMLX, true},
		{"no file", fstest.MapFS{}, fallbackOMLX, false},
		{"broken file", settingsFS(`{`), fallbackOMLX, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s, ok := readOMLXSettings("/Users/ralf", tt.files)
			if ok != tt.ok || s.baseURL() != tt.want {
				t.Errorf("baseURL = %q (ok %v), want %q (ok %v)", s.baseURL(), ok, tt.want, tt.ok)
			}
		})
	}
}

func TestAPIKey(t *testing.T) {
	s, _ := readOMLXSettings("/Users/ralf", settingsFS(`{"auth":{"api_key":" from-file "}}`))
	tests := []struct {
		name string
		env  string
		s    omlxSettings
		want string
	}{
		{"environment wins", "from-env", s, "from-env"},
		{"settings file", "", s, "from-file"},
		{"none at all is allowed", "", omlxSettings{}, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.s.apiKey(func(string) string { return tt.env }); got != tt.want {
				t.Errorf("apiKey = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestClassify(t *testing.T) {
	tests := []struct {
		code int
		want error
	}{
		{503, readiness.ErrStarting},
		{401, readiness.ErrUnauthorized},
		{403, readiness.ErrUnauthorized},
	}
	for _, tt := range tests {
		err := classify(fmt.Errorf("get /health: %w", &omlx.StatusError{Code: tt.code}))
		if !errors.Is(err, tt.want) {
			t.Errorf("HTTP %d → %v, want %v", tt.code, err, tt.want)
		}
	}
	plain := errors.New("connection refused")
	if got := classify(plain); got != plain {
		t.Errorf("network error changed to %v", got)
	}
	if got := classify(nil); got != nil {
		t.Errorf("nil changed to %v", got)
	}
}

func TestToServerModels(t *testing.T) {
	got := toServerModels([]omlx.Model{
		{ID: "a", EstimatedSize: 42, ExternalEngines: []omlx.ExternalEngine{{ID: "splash", Compatible: true}}},
		{ID: "b"},
		{ID: "c", ExternalEngines: []omlx.ExternalEngine{{ID: "splash", DraftRepo: "incoai/X-DFlash2"}}},
	})
	if len(got) != 3 || got[0].Splash == nil || !got[0].Splash.Compatible || got[0].SizeBytes != 42 || got[1].Splash != nil ||
		got[2].Splash == nil || got[2].Splash.DraftRepo != "incoai/X-DFlash2" {
		t.Errorf("toServerModels = %+v", got)
	}
}
