// Package demo runs the live race for the video: the same prompt first on
// oMLX's own engine, then on Splash, with the text streaming visibly.
package demo

import (
	"context"
	"errors"
	"fmt"
	"time"
	"unicode/utf8"

	"github.com/ralfklemmer/splash-bench/internal/bench"
)

// TextStreamer streams a chat completion and hands every text chunk over.
type TextStreamer interface {
	StreamText(ctx context.Context, req bench.ChatRequest, onDelta func(bench.Delta)) (bench.Usage, error)
}

// Display shows the race to the audience.
type Display interface {
	// Phase announces preparation work that does not count (switch, unload, warm-up).
	Phase(engine bench.Engine, text string)
	StartRun(engine bench.Engine)
	Text(delta bench.Delta)
	Progress(p Progress)
	FinishRun(result RunResult)
	Notice(text string)
	Summary(s Summary)
}

// Engines are raced in this order: baseline first.
var Engines = []bench.Engine{bench.EngineOMLX, bench.EngineSplash}

// EngineName is the label the audience sees.
func EngineName(e bench.Engine) string {
	if e == bench.EngineSplash {
		return "Splash"
	}
	return e.Label()
}

// Config describes one race.
type Config struct {
	Model        string
	Prompt       string
	MaxTokens    int
	WarmupTokens int
}

// Progress is the live state of a running measurement. Tokens is an
// estimate from the streamed characters; exact counts arrive at the end.
type Progress struct {
	Engine          bench.Engine
	Elapsed         time.Duration
	FirstToken      time.Duration // 0 while waiting for the first token
	Tokens          float64
	TokensPerSecond float64
}

// RunResult is the measurement of one engine.
type RunResult struct {
	Engine bench.Engine
	Sample bench.Sample
	Total  time.Duration
}

// Summary compares the engines of one race.
type Summary struct {
	Model string
	Runs  []RunResult
}

// Factor is how many times faster the last engine generates than the first.
func (s Summary) Factor() float64 {
	if len(s.Runs) < 2 {
		return 0
	}
	base := s.Runs[0].Sample.DecodeTokensPerSecond()
	if base == 0 {
		return 0
	}
	return s.Runs[len(s.Runs)-1].Sample.DecodeTokensPerSecond() / base
}

// Race switches the model's engine, measures and restores the original
// engine afterwards, also on errors and cancellation.
type Race struct {
	control  bench.EngineControl
	streamer TextStreamer
	clock    bench.Clock
	display  Display
	refresh  time.Duration
}

func NewRace(control bench.EngineControl, streamer TextStreamer, clock bench.Clock, display Display) *Race {
	return &Race{control: control, streamer: streamer, clock: clock, display: display, refresh: 100 * time.Millisecond}
}

// Run executes the race and returns the comparison.
func (r *Race) Run(ctx context.Context, cfg Config) (summary Summary, err error) {
	original, err := r.control.CurrentEngine(ctx, cfg.Model)
	if err != nil {
		return Summary{}, fmt.Errorf("read engine of %s: %w", cfg.Model, err)
	}
	defer func() {
		if ctx.Err() != nil {
			r.display.Notice("Abgebrochen.")
		}
		err = errors.Join(err, r.restore(context.WithoutCancel(ctx), cfg.Model, original))
	}()

	summary = Summary{Model: cfg.Model}
	estimator := newTokenEstimator()
	for _, engine := range Engines {
		result, err := r.runEngine(ctx, cfg, engine, estimator)
		if err != nil {
			return summary, err
		}
		summary.Runs = append(summary.Runs, result)
	}
	r.display.Summary(summary)
	return summary, nil
}

func (r *Race) runEngine(ctx context.Context, cfg Config, engine bench.Engine, est *tokenEstimator) (RunResult, error) {
	if err := r.prepare(ctx, cfg, engine); err != nil {
		return RunResult{}, err
	}
	r.display.StartRun(engine)
	result, err := r.measure(ctx, cfg, engine, est)
	if err != nil {
		return RunResult{}, err
	}
	r.display.FinishRun(result)
	return result, nil
}

// prepare activates the engine and loads the model with a short warm-up,
// so that loading time does not count.
func (r *Race) prepare(ctx context.Context, cfg Config, engine bench.Engine) error {
	name := EngineName(engine)
	r.display.Phase(engine, "Engine "+name+" aktivieren …")
	if err := r.control.SetEngine(ctx, cfg.Model, engine); err != nil {
		return fmt.Errorf("set engine %s for %s: %w", name, cfg.Model, err)
	}
	r.display.Phase(engine, "Modell entladen …")
	if err := r.control.Unload(ctx, cfg.Model); err != nil {
		return fmt.Errorf("unload %s: %w", cfg.Model, err)
	}
	r.display.Phase(engine, "Modell laden und aufwärmen (zählt nicht) …")
	warmup := bench.ChatRequest{Model: cfg.Model, Prompt: cfg.Prompt, MaxTokens: cfg.WarmupTokens}
	if _, err := r.streamer.StreamText(ctx, warmup, func(bench.Delta) {}); err != nil {
		return fmt.Errorf("warm up %s/%s: %w", cfg.Model, name, err)
	}
	return nil
}

func (r *Race) measure(ctx context.Context, cfg Config, engine bench.Engine, est *tokenEstimator) (RunResult, error) {
	tracker := runTracker{engine: engine, start: r.clock.Now(), est: est}
	req := bench.ChatRequest{Model: cfg.Model, Prompt: cfg.Prompt, MaxTokens: cfg.MaxTokens}
	usage, err := r.streamer.StreamText(ctx, req, func(d bench.Delta) {
		now := r.clock.Now()
		tracker.observe(now, d)
		r.display.Text(d)
		if now.Sub(tracker.lastProgress) >= r.refresh {
			tracker.lastProgress = now
			r.display.Progress(tracker.progress(now))
		}
	})
	if err != nil {
		return RunResult{}, fmt.Errorf("stream %s/%s: %w", cfg.Model, EngineName(engine), err)
	}
	sample, err := bench.NewSample(cfg.Model, engine, "demo", tracker.start, tracker.first, tracker.last, usage)
	if err != nil {
		return RunResult{}, err
	}
	est.calibrate(tracker.chars, usage.CompletionTokens)
	return RunResult{Engine: engine, Sample: sample, Total: tracker.last.Sub(tracker.start)}, nil
}

func (r *Race) restore(ctx context.Context, model string, original bench.Engine) error {
	r.display.Notice("Engine von " + model + " wird auf " + EngineName(original) + " zurückgesetzt …")
	if err := r.control.SetEngine(ctx, model, original); err != nil {
		return fmt.Errorf("restore engine %s for %s: %w", EngineName(original), model, err)
	}
	if err := r.control.Unload(ctx, model); err != nil {
		return fmt.Errorf("unload %s after demo: %w", model, err)
	}
	r.display.Notice("Engine zurückgesetzt: " + EngineName(original))
	return nil
}

// runTracker follows one streamed completion.
type runTracker struct {
	engine       bench.Engine
	est          *tokenEstimator
	start        time.Time
	first, last  time.Time
	lastProgress time.Time
	chars        int
}

func (t *runTracker) observe(now time.Time, d bench.Delta) {
	if t.first.IsZero() {
		t.first = now
	}
	t.last = now
	t.chars += utf8.RuneCountInString(d.Content) + utf8.RuneCountInString(d.Reasoning)
}

func (t *runTracker) progress(now time.Time) Progress {
	p := Progress{Engine: t.engine, Elapsed: now.Sub(t.start)}
	if t.first.IsZero() {
		return p
	}
	p.FirstToken = t.first.Sub(t.start)
	p.Tokens = t.est.tokens(t.chars)
	if decode := now.Sub(t.first).Seconds(); decode > 0 && p.Tokens > 1 {
		p.TokensPerSecond = (p.Tokens - 1) / decode
	}
	return p
}

// tokenEstimator converts streamed characters into an approximate token
// count. Chunks are not tokens (Splash may send several tokens at once),
// so characters are counted and calibrated with the server's exact count
// after each run; the second engine thus gets a precise live estimate.
type tokenEstimator struct {
	charsPerToken float64
}

const defaultCharsPerToken = 3.5

func newTokenEstimator() *tokenEstimator {
	return &tokenEstimator{charsPerToken: defaultCharsPerToken}
}

func (e *tokenEstimator) tokens(chars int) float64 {
	return float64(chars) / e.charsPerToken
}

func (e *tokenEstimator) calibrate(chars, tokens int) {
	if chars > 0 && tokens > 0 {
		e.charsPerToken = float64(chars) / float64(tokens)
	}
}
