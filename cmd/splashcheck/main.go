// Command splashcheck diagnoses whether this Mac is ready for Splash in
// oMLX and prints what is missing, with ready-to-copy commands.
package main

import (
	"context"
	"flag"
	"fmt"
	"net/http"
	"os"
	"time"

	"github.com/ralfklemmer/splash-bench/internal/check"
	"github.com/ralfklemmer/splash-bench/internal/omlx"
)

func main() {
	code, err := run()
	if err != nil {
		fmt.Fprintln(os.Stderr, "Fehler:", err)
		os.Exit(2)
	}
	os.Exit(code)
}

func run() (int, error) {
	baseURL := flag.String("base-url", "http://localhost:8000", "oMLX-Server")
	noColor := flag.Bool("no-color", false, "ohne Farben ausgeben")
	flag.Parse()

	home, err := os.UserHomeDir()
	if err != nil {
		return 0, fmt.Errorf("find home directory: %w", err)
	}
	apiKey := os.Getenv("OMLX_API_KEY")
	client := omlx.NewClient(*baseURL, apiKey, &http.Client{Timeout: 10 * time.Second})
	checker := check.NewChecker(check.ExecRunner{}, os.DirFS("/"), serverAdapter{client}, check.Config{
		BaseURL:   *baseURL,
		Home:      home,
		APIKeySet: apiKey != "",
	})

	report := checker.Run(context.Background())
	renderer := check.Renderer{Color: !*noColor && os.Getenv("NO_COLOR") == "" && isTerminal(os.Stdout)}
	if err := renderer.Render(os.Stdout, report); err != nil {
		return 0, fmt.Errorf("print report: %w", err)
	}
	if report.Count(check.StatusFail) > 0 {
		return 1, nil
	}
	return 0, nil
}

// serverAdapter maps the oMLX client onto the port of package check.
type serverAdapter struct {
	client *omlx.Client
}

func (a serverAdapter) Health(ctx context.Context) error {
	return a.client.Health(ctx)
}

func (a serverAdapter) Models(ctx context.Context) ([]check.ServerModel, error) {
	models, err := a.client.Models(ctx)
	if err != nil {
		return nil, err
	}
	result := make([]check.ServerModel, 0, len(models))
	for _, m := range models {
		result = append(result, toServerModel(m))
	}
	return result, nil
}

func toServerModel(m omlx.Model) check.ServerModel {
	sm := check.ServerModel{ID: m.ID}
	if m.Settings.ExternalEngine != nil {
		sm.Engine = *m.Settings.ExternalEngine
	}
	for _, e := range m.ExternalEngines {
		if e.ID == "splash" {
			sm.Splash = &check.SplashSupport{Compatible: e.Compatible, Reason: e.Reason}
		}
	}
	return sm
}

func isTerminal(f *os.File) bool {
	info, err := f.Stat()
	return err == nil && info.Mode()&os.ModeCharDevice != 0
}
