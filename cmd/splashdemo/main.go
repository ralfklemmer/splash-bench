// Command splashdemo races oMLX's own engine against Splash on one model
// for a live recording: the text streams visibly, a status line shows the
// tokens per second, and the model's original engine is restored at the end.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/ralfklemmer/splash-bench/internal/demo"
	"github.com/ralfklemmer/splash-bench/internal/omlx"
)

const defaultPrompt = "Schreibe in Go eine Funktion `WordFrequency(text string) map[string]int`, " +
	"die Wörter ohne Beachtung der Groß- und Kleinschreibung zählt und Satzzeichen ignoriert, " +
	"dazu eine Funktion `TopN(freq map[string]int, n int) []string`, die die n häufigsten Wörter " +
	"sortiert zurückgibt. Füge tabellengetriebene Tests hinzu. Gib nur Code aus, ohne Erklärungen."

type systemClock struct{}

func (systemClock) Now() time.Time { return time.Now() }

// display is a demo.Display that must restore the terminal at the end.
type display interface {
	demo.Display
	Close()
}

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "Fehler:", err)
		os.Exit(1)
	}
}

func run() error {
	baseURL := flag.String("base-url", "http://localhost:8000", "oMLX-Server")
	model := flag.String("model", "Qwen3.8-27B-oQ4e-mtp", "Modell-ID in oMLX")
	prompt := flag.String("prompt", defaultPrompt, "Prompt für beide Engines")
	maxTokens := flag.Int("max-tokens", 512, "maximale Ausgabelänge")
	plain := flag.Bool("plain", false, "ohne Vollbild-Anzeige (für Logs und Pipes)")
	flag.Parse()

	apiKey := os.Getenv("OMLX_API_KEY")
	if apiKey == "" {
		return errors.New("OMLX_API_KEY ist nicht gesetzt (Key aus ~/.omlx/settings.json, auth.api_key)")
	}

	// Keep the signal handler installed until the end: further Ctrl-C
	// presses must not kill the program while it restores the engine.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	out := newDisplay(os.Stdout, *plain, *model)
	defer out.Close()

	client := omlx.NewClient(*baseURL, apiKey, &http.Client{Timeout: 30 * time.Minute})
	race := demo.NewRace(client, client, systemClock{}, out)
	_, err := race.Run(ctx, demo.Config{Model: *model, Prompt: *prompt, MaxTokens: *maxTokens, WarmupTokens: 16})
	return err
}

func newDisplay(f *os.File, plain bool, model string) display {
	rows, cols, ok := terminalSize(f)
	if plain || !ok || rows < 10 {
		return demo.NewPlainDisplay(f)
	}
	return demo.NewTerminalDisplay(io.Writer(f), rows, cols, "Splash-Demo · "+model+" · oMLX vs. Splash")
}
