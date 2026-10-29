// Package bench compares the decode performance of oMLX's in-process engine
// with the external Splash engine on the same models and prompts.
package bench

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"time"
)

// Engine names the backend oMLX uses for a model. The zero value is oMLX's
// own in-process engine.
type Engine string

const (
	EngineOMLX   Engine = ""
	EngineSplash Engine = "splash"
)

// Label is the human-readable engine name used in logs and reports.
func (e Engine) Label() string {
	if e == EngineOMLX {
		return "oMLX"
	}
	return string(e)
}

// Prompt is one benchmark scenario.
type Prompt struct {
	Name string
	Text string
}

// ChatRequest is a single streamed chat completion.
type ChatRequest struct {
	Model     string
	Prompt    string
	MaxTokens int
	Thinking  bool
}

// Usage is the token accounting the server reports for a completion.
type Usage struct {
	PromptTokens     int
	CompletionTokens int
}

// Sample is one measured completion.
type Sample struct {
	Model            string        `json:"model"`
	Engine           Engine        `json:"engine"`
	Prompt           string        `json:"prompt"`
	TimeToFirstToken time.Duration `json:"time_to_first_token_ns"`
	DecodeDuration   time.Duration `json:"decode_duration_ns"`
	PromptTokens     int           `json:"prompt_tokens"`
	CompletionTokens int           `json:"completion_tokens"`
}

// ErrTooFewTokens means a completion is too short to derive a decode rate.
var ErrTooFewTokens = errors.New("zu wenige Tokens für eine Messung")

// NewSample builds a Sample from the timestamps of a streamed completion.
// The decode rate covers the tokens after the first one, because the first
// token's latency is the prefill and is reported separately.
func NewSample(model string, engine Engine, prompt string, start, firstChunk, lastChunk time.Time, usage Usage) (Sample, error) {
	if usage.CompletionTokens < 2 || firstChunk.IsZero() || !lastChunk.After(firstChunk) {
		return Sample{}, fmt.Errorf("%s/%s/%s: %d Tokens: %w", model, engine.Label(), prompt, usage.CompletionTokens, ErrTooFewTokens)
	}
	return Sample{
		Model:            model,
		Engine:           engine,
		Prompt:           prompt,
		TimeToFirstToken: firstChunk.Sub(start),
		DecodeDuration:   lastChunk.Sub(firstChunk),
		PromptTokens:     usage.PromptTokens,
		CompletionTokens: usage.CompletionTokens,
	}, nil
}

// DecodeTokensPerSecond is the generation rate after the first token.
func (s Sample) DecodeTokensPerSecond() float64 {
	return float64(s.CompletionTokens-1) / s.DecodeDuration.Seconds()
}

// median returns the median of values, or 0 for an empty slice.
func median(values []float64) float64 {
	if len(values) == 0 {
		return 0
	}
	sorted := append([]float64(nil), values...)
	sort.Float64s(sorted)
	mid := len(sorted) / 2
	if len(sorted)%2 == 1 {
		return sorted[mid]
	}
	return (sorted[mid-1] + sorted[mid]) / 2
}

// EngineControl switches and unloads a model's engine on the server.
type EngineControl interface {
	CurrentEngine(ctx context.Context, model string) (Engine, error)
	SetEngine(ctx context.Context, model string, engine Engine) error
	Unload(ctx context.Context, model string) error
}

// ChatStreamer streams a chat completion and hands every chunk that
// carries generated text to onDelta.
type ChatStreamer interface {
	StreamText(ctx context.Context, req ChatRequest, onDelta func(Delta)) (Usage, error)
}

// Clock supplies the current time.
type Clock interface {
	Now() time.Time
}

// Delta is the generated text of one streamed chunk.
type Delta struct {
	Content   string
	Reasoning string
}

// Empty reports whether the chunk carries no generated text.
func (d Delta) Empty() bool {
	return d.Content == "" && d.Reasoning == ""
}
