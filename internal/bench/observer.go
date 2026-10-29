package bench

import (
	"fmt"
	"io"
	"time"
)

// StepKind names what the runner does next.
type StepKind string

const (
	StepCheck   StepKind = "check"
	StepSwitch  StepKind = "switch"
	StepWarmup  StepKind = "warmup"
	StepMeasure StepKind = "measure"
	StepRestore StepKind = "restore"
)

// Step announces the runner's next action. Prompt, Repeat (from 1) and
// Repeats are set only for StepMeasure; for StepRestore, Engine is the
// engine being restored.
type Step struct {
	Kind    StepKind `json:"kind"`
	Model   string   `json:"model"`
	Engine  Engine   `json:"engine"`
	Prompt  string   `json:"prompt,omitempty"`
	Repeat  int      `json:"repeat,omitempty"`
	Repeats int      `json:"repeats,omitempty"`
}

// Live is the state of the measurement in progress. Tokens is an estimate
// from the streamed characters; exact counts arrive with the Sample.
type Live struct {
	Tokens           float64       `json:"tokens"`
	TimeToFirstToken time.Duration `json:"time_to_first_token_ns"`
	TokensPerSecond  float64       `json:"tokens_per_second"`
}

func newLive(start, first, now time.Time, tokens float64) Live {
	live := Live{Tokens: tokens, TimeToFirstToken: first.Sub(start)}
	if tokens > 1 && now.After(first) {
		live.TokensPerSecond = (tokens - 1) / now.Sub(first).Seconds()
	}
	return live
}

// defaultCharsPerToken is the estimate until a run has been calibrated.
const defaultCharsPerToken = 3.5

// tokenEstimator turns streamed characters into approximate tokens. Chunks
// are not tokens (Splash often sends several at once), so characters are
// counted and calibrated per model and prompt with the server's exact count;
// Splash, measured after oMLX, thus gets a calibrated live estimate.
type tokenEstimator struct {
	charsPerToken map[estimatorKey]float64
}

type estimatorKey struct{ model, prompt string }

func newTokenEstimator() *tokenEstimator {
	return &tokenEstimator{charsPerToken: map[estimatorKey]float64{}}
}

func (e *tokenEstimator) tokens(model, prompt string, chars int) float64 {
	cpt, ok := e.charsPerToken[estimatorKey{model, prompt}]
	if !ok {
		cpt = defaultCharsPerToken
	}
	return float64(chars) / cpt
}

func (e *tokenEstimator) calibrate(model, prompt string, chars, tokens int) {
	if chars > 0 && tokens > 0 {
		e.charsPerToken[estimatorKey{model, prompt}] = float64(chars) / float64(tokens)
	}
}

// Observer follows a benchmark run.
type Observer interface {
	Step(s Step)
	Live(l Live)
	Measured(s Sample)
	Failed(f Failure)
}

// LogObserver writes one progress line per step and measurement.
type LogObserver struct {
	w      io.Writer
	repeat int
}

func NewLogObserver(w io.Writer) *LogObserver {
	return &LogObserver{w: w}
}

func (o *LogObserver) Step(s Step) {
	switch s.Kind {
	case StepCheck:
		fmt.Fprintf(o.w, "%s: prüfe, ob %s das Modell laden kann\n", s.Model, s.Engine.Label())
	case StepSwitch:
		fmt.Fprintf(o.w, "%s: Engine %s aktivieren\n", s.Model, s.Engine.Label())
	case StepWarmup:
		fmt.Fprintf(o.w, "%s/%s: Aufwärmen (Modell laden)\n", s.Model, s.Engine.Label())
	case StepMeasure:
		o.repeat = s.Repeat
	case StepRestore:
		fmt.Fprintf(o.w, "%s: Engine auf %s zurücksetzen\n", s.Model, s.Engine.Label())
	}
}

func (o *LogObserver) Live(Live) {}

func (o *LogObserver) Failed(f Failure) {
	fmt.Fprintf(o.w, "%s/%s: übersprungen – %s\n", f.Model, f.Engine.Label(), f.Message)
}

func (o *LogObserver) Measured(s Sample) {
	fmt.Fprintf(o.w, "%s/%s/%s #%d: TTFT %v, %.1f tok/s (%d Tokens)\n",
		s.Model, s.Engine.Label(), s.Prompt, o.repeat,
		s.TimeToFirstToken.Round(time.Millisecond), s.DecodeTokensPerSecond(), s.CompletionTokens)
}
