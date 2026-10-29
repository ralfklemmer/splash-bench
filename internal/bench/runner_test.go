package bench

import (
	"context"
	"errors"
	"fmt"
	"io"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"
)

// fakeClock advances by one second on every reading.
type fakeClock struct{ now time.Time }

func (c *fakeClock) Now() time.Time {
	c.now = c.now.Add(time.Second)
	return c.now
}

// recorder logs every call to the fakes in order.
type recorder struct{ calls []string }

type fakeControl struct {
	rec        *recorder
	current    Engine
	setErrFor  Engine
	restoreErr error
	sets       int
}

func (f *fakeControl) CurrentEngine(_ context.Context, model string) (Engine, error) {
	f.rec.calls = append(f.rec.calls, "current "+model)
	return f.current, nil
}

func (f *fakeControl) SetEngine(_ context.Context, model string, engine Engine) error {
	f.sets++
	f.rec.calls = append(f.rec.calls, fmt.Sprintf("set %s %s", model, engine.Label()))
	if f.setErrFor != "" && engine == f.setErrFor {
		return errors.New("draft missing")
	}
	if f.restoreErr != nil && engine == f.current && f.sets > 1 {
		return f.restoreErr
	}
	return nil
}

func (f *fakeControl) Unload(_ context.Context, model string) error {
	f.rec.calls = append(f.rec.calls, "unload "+model)
	return nil
}

type fakeStreamer struct {
	rec     *recorder
	chunks  int
	failAt  int // 1-based stream call that fails; 0 never
	streams int
}

// StreamText sends chunks of seven characters: two tokens by the default
// estimate, one token by the fake's exact count.
func (f *fakeStreamer) StreamText(_ context.Context, req ChatRequest, onDelta func(Delta)) (Usage, error) {
	f.streams++
	f.rec.calls = append(f.rec.calls, fmt.Sprintf("stream %s max=%d", req.Model, req.MaxTokens))
	if f.streams == f.failAt {
		return Usage{}, errors.New("connection reset")
	}
	for range f.chunks {
		onDelta(Delta{Content: "abcdefg"})
	}
	return Usage{CompletionTokens: f.chunks}, nil
}

func testPlan() Plan {
	return Plan{
		Models:          []string{"m"},
		Engines:         []Engine{EngineOMLX, EngineSplash},
		Prompts:         []Prompt{{Name: "code", Text: "x"}},
		Repeats:         1,
		MaxTokens:       100,
		WarmupMaxTokens: 8,
	}
}

func TestRunMeasuresEachEngineAndRestores(t *testing.T) {
	rec := &recorder{}
	control := &fakeControl{rec: rec, current: EngineOMLX}
	streamer := &fakeStreamer{rec: rec, chunks: 5}
	runner := NewRunner(control, streamer, &fakeClock{now: t0})

	samples, err := runner.Run(context.Background(), testPlan(), NewLogObserver(io.Discard))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	wantCalls := []string{
		"current m",
		"set m splash", "unload m", "stream m max=1", // check that Splash loads the model
		"set m oMLX", "unload m", "stream m max=8", "stream m max=100",
		"set m splash", "unload m", "stream m max=8", "stream m max=100",
		"set m oMLX", "unload m",
	}
	if !reflect.DeepEqual(rec.calls, wantCalls) {
		t.Errorf("calls =\n%v\nwant\n%v", rec.calls, wantCalls)
	}
	if len(samples) != 2 {
		t.Fatalf("got %d samples, want 2", len(samples))
	}
	// Clock ticks once per second: start, then 5 chunks → TTFT 1s, decode 4s.
	s := samples[1]
	if s.Engine != EngineSplash || s.TimeToFirstToken != time.Second || s.DecodeDuration != 4*time.Second {
		t.Errorf("sample = %+v", s)
	}
}

func TestRunRestoresEngineWhenStreamFails(t *testing.T) {
	rec := &recorder{}
	control := &fakeControl{rec: rec, current: EngineSplash}
	streamer := &fakeStreamer{rec: rec, chunks: 5, failAt: 3} // first measurement fails
	runner := NewRunner(control, streamer, &fakeClock{now: t0})
	obs := &recordingObserver{}

	if _, err := runner.Run(context.Background(), testPlan(), obs); err != nil {
		t.Fatalf("a failing model must not end the run: %v", err)
	}
	want := []Failure{{Model: "m", Engine: EngineOMLX, Stage: StepMeasure, Message: "stream m/oMLX/code: connection reset"}}
	if !reflect.DeepEqual(obs.failures, want) {
		t.Errorf("failures = %+v, want %+v", obs.failures, want)
	}
	last := rec.calls[len(rec.calls)-2:]
	if !reflect.DeepEqual(last, []string{"set m splash", "unload m"}) {
		t.Errorf("last calls = %v, want restore to splash", last)
	}
}

func TestRunReportsSetEngineFailureAndRestoreFailure(t *testing.T) {
	rec := &recorder{}
	restoreErr := errors.New("server gone")
	control := &fakeControl{rec: rec, current: EngineOMLX, setErrFor: EngineSplash, restoreErr: restoreErr}
	streamer := &fakeStreamer{rec: rec, chunks: 5}
	runner := NewRunner(control, streamer, &fakeClock{now: t0})

	samples, err := runner.Run(context.Background(), testPlan(), NewLogObserver(io.Discard))
	if err == nil {
		t.Fatal("expected error")
	}
	if !errors.Is(err, restoreErr) {
		t.Errorf("err = %v, want it to include the restore failure", err)
	}
	if len(samples) != 0 {
		t.Errorf("samples = %+v, want none: the check fails before measuring", samples)
	}
}

func TestRunFailsOnEmptyStream(t *testing.T) {
	rec := &recorder{}
	control := &fakeControl{rec: rec, current: EngineOMLX}
	streamer := &fakeStreamer{rec: rec, chunks: 0}
	runner := NewRunner(control, streamer, &fakeClock{now: t0})

	obs := &recordingObserver{}
	if _, err := runner.Run(context.Background(), testPlan(), obs); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(obs.failures) != 1 || !strings.Contains(obs.failures[0].Message, ErrTooFewTokens.Error()) {
		t.Errorf("failures = %+v, want ErrTooFewTokens", obs.failures)
	}
}

func TestRunSkipsFailingModelAndGoesOn(t *testing.T) {
	rec := &recorder{}
	control := &fakeControl{rec: rec, current: EngineOMLX}
	streamer := &fakeStreamer{rec: rec, chunks: 5, failAt: 1} // Splash cannot load the first model
	runner := NewRunner(control, streamer, &fakeClock{now: t0})
	obs := &recordingObserver{}
	plan := testPlan()
	plan.Models = []string{"broken", "good"}

	samples, err := runner.Run(context.Background(), plan, obs)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(obs.failures) != 1 || obs.failures[0].Model != "broken" || obs.failures[0].Engine != EngineSplash || obs.failures[0].Stage != StepCheck {
		t.Errorf("failures = %+v, want the Splash check of broken", obs.failures)
	}
	var models []string
	for _, s := range samples {
		models = append(models, s.Model+"/"+s.Engine.Label())
	}
	if want := []string{"good/oMLX", "good/splash"}; !reflect.DeepEqual(models, want) {
		t.Errorf("samples = %v, want %v: no oMLX run for a model Splash cannot load", models, want)
	}
	if !slices.Contains(rec.calls, "set broken oMLX") {
		t.Errorf("calls = %v, want broken restored to oMLX", rec.calls)
	}
}

func TestRunStopsWhenCancelled(t *testing.T) {
	rec := &recorder{}
	ctx, cancel := context.WithCancel(context.Background())
	streamer := &cancellingStreamer{fakeStreamer: fakeStreamer{rec: rec, chunks: 5}, cancel: cancel}
	runner := NewRunner(&fakeControl{rec: rec, current: EngineOMLX}, streamer, &fakeClock{now: t0})
	plan := testPlan()
	plan.Models = []string{"a", "b"}

	_, err := runner.Run(ctx, plan, &recordingObserver{})
	if !errors.Is(err, context.Canceled) {
		t.Errorf("err = %v, want context.Canceled", err)
	}
	if slices.Contains(rec.calls, "current b") {
		t.Errorf("calls = %v, want no second model after cancel", rec.calls)
	}
}

// cancellingStreamer cancels the run during the first stream and fails it.
type cancellingStreamer struct {
	fakeStreamer
	cancel context.CancelFunc
}

func (c *cancellingStreamer) StreamText(ctx context.Context, req ChatRequest, onDelta func(Delta)) (Usage, error) {
	c.cancel()
	return Usage{}, ctx.Err()
}

type recordingObserver struct {
	steps    []StepKind
	lives    []Live
	measured int
	failures []Failure
}

func (o *recordingObserver) Step(s Step)      { o.steps = append(o.steps, s.Kind) }
func (o *recordingObserver) Live(l Live)      { o.lives = append(o.lives, l) }
func (o *recordingObserver) Measured(Sample)  { o.measured++ }
func (o *recordingObserver) Failed(f Failure) { o.failures = append(o.failures, f) }

func TestRunReportsStepsAndLiveProgress(t *testing.T) {
	rec := &recorder{}
	runner := NewRunner(&fakeControl{rec: rec, current: EngineOMLX}, &fakeStreamer{rec: rec, chunks: 5}, &fakeClock{now: t0})
	obs := &recordingObserver{}

	if _, err := runner.Run(context.Background(), testPlan(), obs); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	wantSteps := []StepKind{StepCheck, StepSwitch, StepWarmup, StepMeasure, StepSwitch, StepWarmup, StepMeasure, StepRestore}
	if !reflect.DeepEqual(obs.steps, wantSteps) {
		t.Errorf("steps = %v, want %v", obs.steps, wantSteps)
	}
	if obs.measured != 2 {
		t.Errorf("measured = %d, want 2", obs.measured)
	}
	if len(obs.lives) != 10 {
		t.Fatalf("got %d live updates, want 5 per measurement", len(obs.lives))
	}
	// Clock ticks once per second: first chunk after 1s, fifth after 4 more.
	// oMLX runs uncalibrated: 35 characters / 3.5 = 10 tokens in 4 s.
	omlx := obs.lives[4]
	if omlx.Tokens != 10 || omlx.TimeToFirstToken != time.Second || omlx.TokensPerSecond != 2.25 {
		t.Errorf("last oMLX live = %+v", omlx)
	}
	// Splash is calibrated by oMLX's exact count: 7 characters per token.
	splash := obs.lives[9]
	if splash.Tokens != 5 || splash.TokensPerSecond != 1 {
		t.Errorf("last Splash live = %+v, want the calibrated estimate", splash)
	}
}
