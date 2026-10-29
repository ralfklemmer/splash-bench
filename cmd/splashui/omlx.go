package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"net"
	"net/http"
	"os"
	"path"
	"strconv"
	"strings"

	"github.com/ralfklemmer/splash-bench/internal/check"
	"github.com/ralfklemmer/splash-bench/internal/omlx"
	"github.com/ralfklemmer/splash-bench/internal/readiness"
)

const (
	settingsPath  = "~/.omlx/settings.json"
	fallbackOMLX  = "http://localhost:8000"
	settingsLabel = settingsPath + " (server.host, server.port)"
)

// omlxSettings is the part of oMLX's settings file the app uses.
type omlxSettings struct {
	Server struct {
		Host string `json:"host"`
		Port int    `json:"port"`
	} `json:"server"`
	Auth struct {
		APIKey string `json:"api_key"`
	} `json:"auth"`
}

// readOMLXSettings reads ~/.omlx/settings.json; ok is false when it is
// missing or unreadable.
func readOMLXSettings(home string, files fs.FS) (s omlxSettings, ok bool) {
	data, err := fs.ReadFile(files, strings.TrimPrefix(path.Join(home, ".omlx", "settings.json"), "/"))
	if err != nil {
		return omlxSettings{}, false
	}
	if err := json.Unmarshal(data, &s); err != nil {
		return omlxSettings{}, false
	}
	return s, true
}

// baseURL is where oMLX listens according to its settings. A server bound
// to all interfaces is reached via localhost.
func (s omlxSettings) baseURL() string {
	if s.Server.Port == 0 {
		return fallbackOMLX
	}
	host := s.Server.Host
	if host == "" || host == "0.0.0.0" || host == "::" {
		host = "localhost"
	}
	return "http://" + net.JoinHostPort(host, strconv.Itoa(s.Server.Port))
}

// apiKey prefers OMLX_API_KEY over the settings file. An empty key is
// fine: oMLX accepts local requests without one unless configured otherwise.
func (s omlxSettings) apiKey(getenv func(string) string) string {
	if key := strings.TrimSpace(getenv("OMLX_API_KEY")); key != "" {
		return key
	}
	return strings.TrimSpace(s.Auth.APIKey)
}

// modelDir is the first oMLX model directory as a shell path; oMLX's
// default when the settings name none or cannot be read.
func modelDir(home string) string {
	dir := check.DefaultModelDir(home)
	if dirs, err := check.ReadModelDirs(os.DirFS("/"), home); err == nil && len(dirs) > 0 {
		dir = dirs[0]
	}
	return check.ShellPath(dir, home)
}

// readinessServer adapts the oMLX client to the readiness checks.
type readinessServer struct {
	client *omlx.Client
}

func (a readinessServer) Health(ctx context.Context) error {
	return classify(a.client.Health(ctx))
}

func (a readinessServer) Models(ctx context.Context) ([]readiness.ServerModel, error) {
	models, err := a.client.Models(ctx)
	if err != nil {
		return nil, classify(err)
	}
	return toServerModels(models), nil
}

// classify maps oMLX answers onto the errors the checks explain.
func classify(err error) error {
	var status *omlx.StatusError
	if !errors.As(err, &status) {
		return err
	}
	switch status.Code {
	case http.StatusServiceUnavailable:
		return fmt.Errorf("%w: %w", readiness.ErrStarting, err)
	case http.StatusUnauthorized, http.StatusForbidden:
		return fmt.Errorf("%w: %w", readiness.ErrUnauthorized, err)
	}
	return err
}

func toServerModels(models []omlx.Model) []readiness.ServerModel {
	out := make([]readiness.ServerModel, 0, len(models))
	for _, m := range models {
		sm := readiness.ServerModel{ID: m.ID, SizeBytes: m.EstimatedSize}
		for _, e := range m.ExternalEngines {
			if e.ID == "splash" {
				sm.Splash = &readiness.SplashSupport{Compatible: e.Compatible, Reason: e.Reason, DraftRepo: e.DraftRepo}
			}
		}
		out = append(out, sm)
	}
	return out
}
