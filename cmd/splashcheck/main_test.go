package main

import (
	"testing"

	"github.com/ralfklemmer/splash-bench/internal/omlx"
)

func TestToServerModel(t *testing.T) {
	splash := "splash"
	tests := []struct {
		name string
		in   omlx.Model
		want string // engine|compatible|reason, "-" for no Splash info
	}{
		{"splash active", omlx.Model{ID: "a", Settings: omlx.ModelSettings{ExternalEngine: &splash},
			ExternalEngines: []omlx.ExternalEngine{{ID: "splash", Compatible: true}}}, "splash|true|"},
		{"incompatible", omlx.Model{ID: "b", ExternalEngines: []omlx.ExternalEngine{{ID: "splash", Reason: "nur Qwen"}}}, "|false|nur Qwen"},
		{"no info", omlx.Model{ID: "c"}, "-"},
	}
	for _, tt := range tests {
		got := toServerModel(tt.in)
		desc := "-"
		if got.Splash != nil {
			desc = got.Engine + "|" + map[bool]string{true: "true", false: "false"}[got.Splash.Compatible] + "|" + got.Splash.Reason
		}
		if got.ID != tt.in.ID || desc != tt.want {
			t.Errorf("%s: got %s, want %s", tt.name, desc, tt.want)
		}
	}
}
