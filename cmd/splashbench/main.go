// Command splashbench compares oMLX's in-process engine with Splash on the
// same models. It switches each model's engine through the oMLX admin API,
// measures streamed completions and restores the original setting.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"time"

	"github.com/ralfklemmer/splash-bench/internal/bench"
	"github.com/ralfklemmer/splash-bench/internal/omlx"
)

type systemClock struct{}

func (systemClock) Now() time.Time { return time.Now() }

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "Fehler:", err)
		os.Exit(1)
	}
}

func run() error {
	baseURL := flag.String("base-url", "http://localhost:8000", "oMLX-Server")
	models := flag.String("models", "Qwen3.6-35B-A3B-oQ4e-mtp,Qwen3.8-27B-oQ4e-mtp", "Modelle, kommagetrennt")
	repeats := flag.Int("repeats", 3, "Messläufe pro Prompt und Engine")
	maxTokens := flag.Int("max-tokens", 512, "maximale Ausgabelänge pro Lauf")
	thinking := flag.Bool("thinking", false, "Reasoning (enable_thinking) einschalten")
	out := flag.String("out", "results.json", "Datei für die Rohmessungen")
	flag.Parse()

	apiKey := os.Getenv("OMLX_API_KEY")
	if apiKey == "" {
		return errors.New("OMLX_API_KEY ist nicht gesetzt")
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	client := omlx.NewClient(*baseURL, apiKey, &http.Client{Timeout: 30 * time.Minute})
	runner := bench.NewRunner(client, client, systemClock{})
	plan := bench.Plan{
		Models:          strings.Split(*models, ","),
		Engines:         []bench.Engine{bench.EngineOMLX, bench.EngineSplash},
		Prompts:         bench.DefaultPrompts,
		Repeats:         *repeats,
		MaxTokens:       *maxTokens,
		WarmupMaxTokens: 32,
		Thinking:        *thinking,
	}

	samples, runErr := runner.Run(ctx, plan, bench.NewLogObserver(os.Stderr))
	if err := writeSamples(*out, samples); err != nil {
		return errors.Join(runErr, err)
	}
	if err := bench.WriteMarkdown(os.Stdout, bench.Summarize(samples)); err != nil {
		return errors.Join(runErr, err)
	}
	return runErr
}

func writeSamples(path string, samples []bench.Sample) error {
	data, err := json.MarshalIndent(samples, "", "  ")
	if err != nil {
		return fmt.Errorf("encode samples: %w", err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}
	return nil
}
