package compare

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/ralfklemmer/splash-bench/internal/bench"
	"github.com/ralfklemmer/splash-bench/internal/check"
)

var t0 = time.Date(2026, 10, 24, 20, 0, 0, 0, time.UTC)

type fixedClock struct{}

func (fixedClock) Now() time.Time { return t0 }

type fixedSystem struct{}

func (fixedSystem) System(context.Context) check.System {
	return check.System{Chip: "M5 Max", MemoryGB: 128, MacOS: "27.0.1", Splash: "1.3.2"}
}

type fakeStore struct {
	saved []Result
	err   error
}

func (f *fakeStore) Save(r Result) error {
	if f.err != nil {
		return f.err
	}
	f.saved = append(f.saved, r)
	return nil
}

// fakeRunner reports one sample per engine and prompt. With block set it
// stops after the first sample and waits for cancellation.
type fakeRunner struct {
	plan    bench.Plan
	err     error
	block   bool
	failure *bench.Failure // reported before the samples
}

func sample(model string, engine bench.Engine, prompt string, tps float64) bench.Sample {
	// 101 tokens: 100 after the first one, decoded in 100/tps seconds.
	return bench.Sample{Model: model, Engine: engine, Prompt: prompt, CompletionTokens: 101,
		TimeToFirstToken: 50 * time.Millisecond, DecodeDuration: time.Duration(100 / tps * float64(time.Second))}
}

func (f *fakeRunner) Run(ctx context.Context, plan bench.Plan, obs bench.Observer) ([]bench.Sample, error) {
	f.plan = plan
	if f.failure != nil {
		obs.Failed(*f.failure)
	}
	var samples []bench.Sample
	rates := map[bench.Engine]float64{bench.EngineOMLX: 50, bench.EngineSplash: 100}
	for _, model := range plan.Models {
		for _, engine := range plan.Engines {
			obs.Step(bench.Step{Kind: bench.StepSwitch, Model: model, Engine: engine})
			for _, p := range plan.Prompts {
				obs.Step(bench.Step{Kind: bench.StepMeasure, Model: model, Engine: engine, Prompt: p.Name, Repeat: 1, Repeats: plan.Repeats})
				obs.Live(bench.Live{Tokens: 10, TokensPerSecond: rates[engine]})
				s := sample(model, engine, p.Name, rates[engine])
				obs.Measured(s)
				samples = append(samples, s)
				if f.block {
					<-ctx.Done()
					return samples, ctx.Err()
				}
			}
		}
	}
	return samples, f.err
}

func newTestSession(r Runner, st Store) *Session {
	return NewSession(r, st, fixedSystem{}, fixedClock{})
}

func validRequest() Request {
	return Request{Models: []string{"Qwen3.8-27B-oQ4e-mtp"}, Prompts: []string{"code", "translate"}, Repeats: 1}
}

func TestStartRejectsInvalidRequests(t *testing.T) {
	tests := []struct {
		name string
		req  Request
		want string
	}{
		{"no model", Request{Prompts: []string{"code"}, Repeats: 1}, "kein Modell"},
		{"no prompt", Request{Models: []string{"m"}, Repeats: 1}, "keine Aufgabe"},
		{"zero repeats", Request{Models: []string{"m"}, Prompts: []string{"code"}}, "Durchläufe"},
		{"too many repeats", Request{Models: []string{"m"}, Prompts: []string{"code"}, Repeats: MaxRepeats + 1}, "Durchläufe"},
		{"unknown prompt", Request{Models: []string{"m"}, Prompts: []string{"poem"}, Repeats: 1}, "unbekannte Aufgabe"},
		{"duplicate model", Request{Models: []string{"m", "m"}, Prompts: []string{"code"}, Repeats: 1}, "doppelt"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := newTestSession(&fakeRunner{}, &fakeStore{})
			err := s.Start(tt.req)
			if !errors.Is(err, ErrInvalid) || !strings.Contains(err.Error(), tt.want) {
				t.Errorf("err = %v, want ErrInvalid mentioning %q", err, tt.want)
			}
			if s.View().Phase != PhaseIdle {
				t.Errorf("phase = %s, want idle", s.View().Phase)
			}
		})
	}
}

func TestRunMeasuresAndStoresResult(t *testing.T) {
	runner, st := &fakeRunner{}, &fakeStore{}
	s := newTestSession(runner, st)

	if err := s.Start(validRequest()); err != nil {
		t.Fatalf("Start: %v", err)
	}
	s.Wait()

	if names := []string{runner.plan.Prompts[0].Name, runner.plan.Prompts[1].Name}; !reflect.DeepEqual(names, []string{"code", "translate"}) {
		t.Errorf("plan prompts = %v", names)
	}
	if runner.plan.MaxTokens != MaxTokens || !reflect.DeepEqual(runner.plan.Engines, []bench.Engine{bench.EngineOMLX, bench.EngineSplash}) {
		t.Errorf("plan = %+v", runner.plan)
	}

	v := s.View()
	if v.Phase != PhaseDone || v.Source != SourceLive || v.Done != 4 || v.Total != 4 || v.Error != "" || v.Step != nil {
		t.Errorf("view = %+v", v)
	}
	if len(v.Comparison) != 1 || v.Comparison[0].Verdict != bench.VerdictSplash || v.Comparison[0].MeanSpeedup < 1.99 {
		t.Errorf("comparison = %+v", v.Comparison)
	}
	if len(st.saved) != 1 || st.saved[0].System.Chip != "M5 Max" || !st.saved[0].MeasuredAt.Equal(t0) {
		t.Fatalf("saved = %+v", st.saved)
	}
	if v.PreviousAt == nil || !v.PreviousAt.Equal(t0) {
		t.Errorf("previous_at = %v, want the new result", v.PreviousAt)
	}
	if shown, ok := s.Shown(); !ok || len(shown.Samples) != 4 {
		t.Errorf("Shown = %+v, %v", shown, ok)
	}
}

func TestFailedModelIsShownAndStored(t *testing.T) {
	failure := bench.Failure{Model: "broken", Engine: bench.EngineSplash, Stage: bench.StepWarmup, Message: "error: tie_word_embeddings must be false"}
	st := &fakeStore{}
	s := newTestSession(&fakeRunner{failure: &failure}, st)
	if err := s.Start(validRequest()); err != nil {
		t.Fatalf("Start: %v", err)
	}
	s.Wait()

	v := s.View()
	if v.Error != "" || len(v.Failures) != 1 || v.Failures[0].Model != "broken" || v.Failures[0].Title != "Splash konnte das Modell nicht laden" {
		t.Errorf("view error %q, failures %+v", v.Error, v.Failures)
	}
	if len(st.saved) != 1 || !reflect.DeepEqual(st.saved[0].Failures, []bench.Failure{failure}) {
		t.Fatalf("saved = %+v", st.saved)
	}
	if got := st.saved[0].models(); !reflect.DeepEqual(got, []string{"Qwen3.8-27B-oQ4e-mtp", "broken"}) {
		t.Errorf("models of stored result = %v, want the failed one too", got)
	}
}

func TestStartWhileRunningIsBusy(t *testing.T) {
	s := newTestSession(&fakeRunner{block: true}, &fakeStore{})
	if err := s.Start(validRequest()); err != nil {
		t.Fatalf("Start: %v", err)
	}
	if err := s.Start(validRequest()); !errors.Is(err, ErrBusy) {
		t.Errorf("second Start = %v, want ErrBusy", err)
	}
	if err := s.ShowPrevious(); !errors.Is(err, ErrBusy) {
		t.Errorf("ShowPrevious while running = %v, want ErrBusy", err)
	}
	s.Cancel()
	s.Wait()
}

func TestCancelKeepsPartialResultWithoutError(t *testing.T) {
	st := &fakeStore{}
	s := newTestSession(&fakeRunner{block: true}, st)
	if err := s.Start(validRequest()); err != nil {
		t.Fatalf("Start: %v", err)
	}
	s.Cancel()
	s.Wait()

	v := s.View()
	if v.Phase != PhaseDone || !v.Cancelled || v.Error != "" || v.Done != 1 {
		t.Errorf("view = %+v", v)
	}
	if len(st.saved) != 1 || !st.saved[0].Cancelled {
		t.Errorf("saved = %+v, want the cancelled partial result", st.saved)
	}
}

func TestRunnerAndStoreErrorsAreShown(t *testing.T) {
	tests := []struct {
		name     string
		runErr   error
		storeErr error
		want     []string
	}{
		{"runner fails", errors.New("draft missing"), nil, []string{"draft missing"}},
		{"store fails", nil, errors.New("disk full"), []string{"nicht gespeichert", "disk full"}},
		{"both fail", errors.New("draft missing"), errors.New("disk full"), []string{"draft missing", "disk full"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := newTestSession(&fakeRunner{err: tt.runErr}, &fakeStore{err: tt.storeErr})
			if err := s.Start(validRequest()); err != nil {
				t.Fatalf("Start: %v", err)
			}
			s.Wait()
			v := s.View()
			for _, w := range tt.want {
				if !strings.Contains(v.Error, w) {
					t.Errorf("error = %q, want it to contain %q", v.Error, w)
				}
			}
			if v.Cancelled {
				t.Error("an error is not a cancellation")
			}
		})
	}
}

func TestShowPrevious(t *testing.T) {
	s := newTestSession(&fakeRunner{}, &fakeStore{})
	if err := s.ShowPrevious(); !errors.Is(err, ErrNoPrevious) {
		t.Fatalf("ShowPrevious without result = %v, want ErrNoPrevious", err)
	}

	measured := t0.Add(-7 * 24 * time.Hour)
	s.Remember(Result{MeasuredAt: measured, Samples: []bench.Sample{
		sample("b", bench.EngineOMLX, "prose", 50),
		sample("b", bench.EngineOMLX, "prose", 52),
		sample("a", bench.EngineSplash, "code", 90),
	}})
	if err := s.ShowPrevious(); err != nil {
		t.Fatalf("ShowPrevious: %v", err)
	}

	v := s.View()
	if v.Phase != PhaseDone || v.Source != SourceArchive || !v.MeasuredAt.Equal(measured) {
		t.Errorf("view = %+v", v)
	}
	if !reflect.DeepEqual(v.Models, []string{"b", "a"}) || !reflect.DeepEqual(v.Prompts, []string{"prose", "code"}) || v.Repeats != 2 {
		t.Errorf("models %v, prompts %v, repeats %d", v.Models, v.Prompts, v.Repeats)
	}

	s.Reset()
	if v := s.View(); v.Phase != PhaseIdle || v.PreviousAt == nil {
		t.Errorf("after reset view = %+v, want idle and the previous result offered", v)
	}
}

func TestSubscribersAreNotified(t *testing.T) {
	s := newTestSession(&fakeRunner{}, &fakeStore{})
	ch, unsubscribe := s.Subscribe()
	defer unsubscribe()

	if err := s.Start(validRequest()); err != nil {
		t.Fatalf("Start: %v", err)
	}
	s.Wait()
	select {
	case <-ch:
	default:
		t.Fatal("no change signal after a run")
	}
}
