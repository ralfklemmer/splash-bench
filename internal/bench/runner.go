package bench

import (
	"context"
	"errors"
	"fmt"
	"time"
	"unicode/utf8"
)

// Plan describes what a benchmark run measures.
type Plan struct {
	Models          []string
	Engines         []Engine
	Prompts         []Prompt
	Repeats         int
	MaxTokens       int
	WarmupMaxTokens int
	Thinking        bool
}

// Runner executes a Plan. For every model it measures each engine in turn
// and restores the model's original engine afterwards, also on failure.
type Runner struct {
	control  EngineControl
	streamer ChatStreamer
	clock    Clock
}

func NewRunner(control EngineControl, streamer ChatStreamer, clock Clock) *Runner {
	return &Runner{control: control, streamer: streamer, clock: clock}
}

// Failure is a model the runner gave up on: an engine could not be
// switched on, could not load the model or broke off a measurement. The
// run goes on with the next model.
type Failure struct {
	Model   string   `json:"model"`
	Engine  Engine   `json:"engine"`
	Stage   StepKind `json:"stage"`
	Message string   `json:"message"`
}

// Run measures all models, reports progress to obs and returns the samples
// collected so far. A model that fails is reported to obs and skipped; Run
// stops only when ctx ends or an engine cannot be restored, since every
// further measurement would then start from a wrong setting.
func (r *Runner) Run(ctx context.Context, plan Plan, obs Observer) ([]Sample, error) {
	run := &benchRun{Runner: r, plan: plan, obs: obs, estimator: newTokenEstimator()}
	var samples []Sample
	for _, model := range plan.Models {
		modelSamples, failure, err := run.model(ctx, model)
		samples = append(samples, modelSamples...)
		if ctx.Err() != nil {
			return samples, errors.Join(failure.asError(), err, ctx.Err())
		}
		if err != nil {
			return samples, errors.Join(failure.asError(), err)
		}
		if failure != nil {
			obs.Failed(failure.Failure)
		}
	}
	return samples, nil
}

// modelError is a Failure together with the error behind it.
type modelError struct {
	Failure
	err error
}

func (e *modelError) Error() string { return e.err.Error() }
func (e *modelError) Unwrap() error { return e.err }

// asError keeps a nil *modelError from becoming a non-nil error.
func (e *modelError) asError() error {
	if e == nil {
		return nil
	}
	return e
}

// stageError marks at which step an engine failed.
type stageError struct {
	stage StepKind
	err   error
}

func (e *stageError) Error() string { return e.err.Error() }
func (e *stageError) Unwrap() error { return e.err }

// benchRun is one execution of a Plan.
type benchRun struct {
	*Runner
	plan      Plan
	obs       Observer
	estimator *tokenEstimator
}

// model measures every engine on one model. failure is the engine that
// failed, if any; err is a failed restore, which ends the whole run.
func (r *benchRun) model(ctx context.Context, model string) (samples []Sample, failure *modelError, err error) {
	original, readErr := r.control.CurrentEngine(ctx, model)
	if readErr != nil {
		return nil, newModelError(model, "", StepSwitch, fmt.Errorf("read engine of %s: %w", model, readErr)), nil
	}
	defer func() {
		err = r.restore(context.WithoutCancel(ctx), model, original)
	}()

	for _, engine := range r.plan.Engines[min(1, len(r.plan.Engines)):] {
		if checkErr := r.check(ctx, model, engine); checkErr != nil {
			return nil, newModelError(model, engine, StepCheck, checkErr), nil
		}
	}
	for _, engine := range r.plan.Engines {
		engineSamples, engineErr := r.engine(ctx, model, engine)
		samples = append(samples, engineSamples...)
		if engineErr != nil {
			stage := StepMeasure
			var se *stageError
			if errors.As(engineErr, &se) {
				stage = se.stage
			}
			return samples, newModelError(model, engine, stage, engineErr), nil
		}
	}
	return samples, nil, nil
}

func newModelError(model string, engine Engine, stage StepKind, err error) *modelError {
	return &modelError{Failure: Failure{Model: model, Engine: engine, Stage: stage, Message: err.Error()}, err: err}
}

// check loads the model once with an engine measured later, so a model
// that engine cannot run is skipped before any time goes into measuring
// the engines before it. Loading is the only reliable test: oMLX reports
// models as compatible that Splash then refuses.
func (r *benchRun) check(ctx context.Context, model string, engine Engine) error {
	r.obs.Step(Step{Kind: StepCheck, Model: model, Engine: engine})
	if err := r.control.SetEngine(ctx, model, engine); err != nil {
		return fmt.Errorf("set engine %s for %s: %w", engine.Label(), model, err)
	}
	if err := r.control.Unload(ctx, model); err != nil {
		return fmt.Errorf("unload %s: %w", model, err)
	}
	probe := ChatRequest{Model: model, Prompt: r.plan.Prompts[0].Text, MaxTokens: 1, Thinking: r.plan.Thinking}
	if _, err := r.streamer.StreamText(ctx, probe, func(Delta) {}); err != nil {
		return fmt.Errorf("load %s/%s: %w", model, engine.Label(), err)
	}
	return nil
}

func (r *benchRun) engine(ctx context.Context, model string, engine Engine) ([]Sample, error) {
	r.obs.Step(Step{Kind: StepSwitch, Model: model, Engine: engine})
	if err := r.control.SetEngine(ctx, model, engine); err != nil {
		return nil, &stageError{StepSwitch, fmt.Errorf("set engine %s for %s: %w", engine.Label(), model, err)}
	}
	if err := r.control.Unload(ctx, model); err != nil {
		return nil, &stageError{StepSwitch, fmt.Errorf("unload %s: %w", model, err)}
	}

	r.obs.Step(Step{Kind: StepWarmup, Model: model, Engine: engine})
	warmup := ChatRequest{Model: model, Prompt: r.plan.Prompts[0].Text, MaxTokens: r.plan.WarmupMaxTokens, Thinking: r.plan.Thinking}
	if _, err := r.streamer.StreamText(ctx, warmup, func(Delta) {}); err != nil {
		return nil, &stageError{StepWarmup, fmt.Errorf("warm up %s/%s: %w", model, engine.Label(), err)}
	}

	var samples []Sample
	for _, prompt := range r.plan.Prompts {
		for i := 1; i <= r.plan.Repeats; i++ {
			r.obs.Step(Step{Kind: StepMeasure, Model: model, Engine: engine, Prompt: prompt.Name, Repeat: i, Repeats: r.plan.Repeats})
			sample, err := r.measure(ctx, model, engine, prompt)
			if err != nil {
				return samples, err
			}
			r.obs.Measured(sample)
			samples = append(samples, sample)
		}
	}
	return samples, nil
}

func (r *benchRun) measure(ctx context.Context, model string, engine Engine, prompt Prompt) (Sample, error) {
	req := ChatRequest{Model: model, Prompt: prompt.Text, MaxTokens: r.plan.MaxTokens, Thinking: r.plan.Thinking}
	start := r.clock.Now()
	var first, last time.Time
	chars := 0
	usage, err := r.streamer.StreamText(ctx, req, func(d Delta) {
		now := r.clock.Now()
		if first.IsZero() {
			first = now
		}
		last = now
		chars += utf8.RuneCountInString(d.Content) + utf8.RuneCountInString(d.Reasoning)
		r.obs.Live(newLive(start, first, now, r.estimator.tokens(model, prompt.Name, chars)))
	})
	if err != nil {
		return Sample{}, fmt.Errorf("stream %s/%s/%s: %w", model, engine.Label(), prompt.Name, err)
	}
	r.estimator.calibrate(model, prompt.Name, chars, usage.CompletionTokens)
	return NewSample(model, engine, prompt.Name, start, first, last, usage)
}

func (r *benchRun) restore(ctx context.Context, model string, original Engine) error {
	r.obs.Step(Step{Kind: StepRestore, Model: model, Engine: original})
	if err := r.control.SetEngine(ctx, model, original); err != nil {
		return fmt.Errorf("restore engine %s for %s: %w", original.Label(), model, err)
	}
	if err := r.control.Unload(ctx, model); err != nil {
		return fmt.Errorf("unload %s after benchmark: %w", model, err)
	}
	return nil
}
