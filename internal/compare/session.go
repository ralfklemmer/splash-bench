package compare

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/ralfklemmer/splash-bench/internal/bench"
	"github.com/ralfklemmer/splash-bench/internal/check"
)

// Fixed measurement settings of the app; splashbench offers them as flags.
const (
	MaxTokens       = 512
	WarmupMaxTokens = 32
	MaxRepeats      = 10
)

var (
	ErrBusy       = errors.New("es läuft bereits ein Vergleich")
	ErrInvalid    = errors.New("ungültige Auswahl")
	ErrNoPrevious = errors.New("es gibt noch keine gespeicherte Messung")
)

// Runner executes a benchmark plan.
type Runner interface {
	Run(ctx context.Context, plan bench.Plan, obs bench.Observer) ([]bench.Sample, error)
}

// Store keeps finished results.
type Store interface {
	Save(r Result) error
}

// SystemReader reads the profile stored with each result.
type SystemReader interface {
	System(ctx context.Context) check.System
}

// Clock supplies the current time.
type Clock interface {
	Now() time.Time
}

// Request is what the user picked in the app.
type Request struct {
	Models  []string `json:"models"`
	Prompts []string `json:"prompts"`
	Repeats int      `json:"repeats"`
}

// Phase is the state of the session.
type Phase string

const (
	PhaseIdle    Phase = "idle"
	PhaseRunning Phase = "running"
	PhaseDone    Phase = "done"
)

// Source tells whether the shown result was just measured or loaded.
type Source string

const (
	SourceLive    Source = "live"
	SourceArchive Source = "archive"
)

// View is everything the app shows, as one snapshot.
type View struct {
	Phase      Phase                   `json:"phase"`
	Source     Source                  `json:"source,omitempty"`
	MeasuredAt *time.Time              `json:"measured_at,omitempty"`
	System     *check.System           `json:"system,omitempty"`
	Models     []string                `json:"models,omitempty"`
	Prompts    []string                `json:"prompts,omitempty"`
	Repeats    int                     `json:"repeats,omitempty"`
	Step       *bench.Step             `json:"step,omitempty"`
	Live       *bench.Live             `json:"live,omitempty"`
	Done       int                     `json:"done"`
	Total      int                     `json:"total"`
	Cancelled  bool                    `json:"cancelled,omitempty"`
	Error      string                  `json:"error,omitempty"`
	Failures   []Problem               `json:"failures,omitempty"`
	Comparison []bench.ModelComparison `json:"comparison"`
	PreviousAt *time.Time              `json:"previous_at,omitempty"`
}

// Session runs at most one comparison at a time and remembers the last
// result. It is safe for concurrent use.
type Session struct {
	runner Runner
	store  Store
	system SystemReader
	clock  Clock

	mu          sync.Mutex
	phase       Phase
	source      Source
	request     Request
	sys         check.System
	measuredAt  time.Time
	step        *bench.Step
	live        *bench.Live
	samples     []bench.Sample
	failures    []bench.Failure
	cancelled   bool
	errText     string
	previous    *Result
	cancel      context.CancelFunc
	finished    chan struct{}
	subscribers map[chan struct{}]struct{}
}

func NewSession(runner Runner, store Store, system SystemReader, clock Clock) *Session {
	return &Session{
		runner:      runner,
		store:       store,
		system:      system,
		clock:       clock,
		phase:       PhaseIdle,
		subscribers: map[chan struct{}]struct{}{},
	}
}

// Remember makes r the result offered as "last measurement".
func (s *Session) Remember(r Result) {
	s.mu.Lock()
	s.previous = &r
	s.mu.Unlock()
	s.notify()
}

// Start validates req and runs the comparison in the background.
func (s *Session) Start(req Request) error {
	if err := validate(req); err != nil {
		return err
	}
	sys := s.system.System(context.Background())

	s.mu.Lock()
	if s.phase == PhaseRunning {
		s.mu.Unlock()
		return ErrBusy
	}
	ctx, cancel := context.WithCancel(context.Background())
	s.phase, s.source, s.request, s.sys = PhaseRunning, SourceLive, req, sys
	s.measuredAt = s.clock.Now()
	s.step, s.live, s.samples, s.failures = nil, nil, nil, nil
	s.cancelled, s.errText = false, ""
	s.cancel = cancel
	s.finished = make(chan struct{})
	finished := s.finished
	s.mu.Unlock()
	s.notify()

	go s.run(ctx, req, finished)
	return nil
}

func (s *Session) run(ctx context.Context, req Request, finished chan struct{}) {
	defer close(finished)
	samples, err := s.runner.Run(ctx, planFor(req), observer{s})

	s.mu.Lock()
	cancelled := ctx.Err() != nil
	s.cancel()
	result := Result{MeasuredAt: s.measuredAt, System: s.sys, Repeats: req.Repeats, Cancelled: cancelled, Samples: samples, Failures: s.failures}
	s.phase, s.step, s.live = PhaseDone, nil, nil
	s.samples, s.cancelled = samples, cancelled
	if err != nil && !(cancelled && errors.Is(err, context.Canceled)) {
		s.errText = err.Error()
	}
	s.mu.Unlock()

	if len(samples) > 0 {
		if err := s.store.Save(result); err != nil {
			s.mu.Lock()
			s.errText = joinText(s.errText, "Ergebnis nicht gespeichert: "+err.Error())
			s.mu.Unlock()
		} else {
			s.mu.Lock()
			s.previous = &result
			s.mu.Unlock()
		}
	}
	s.notify()
}

// Cancel stops the running comparison; the runner still restores the
// engine. Use Wait to block until that has happened.
func (s *Session) Cancel() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.phase == PhaseRunning && s.cancel != nil {
		s.cancel()
	}
}

// Wait blocks until the running comparison, if any, has finished.
func (s *Session) Wait() {
	s.mu.Lock()
	finished := s.finished
	s.mu.Unlock()
	if finished != nil {
		<-finished
	}
}

// Reset returns to the start screen. It does nothing while running.
func (s *Session) Reset() {
	s.mu.Lock()
	if s.phase == PhaseRunning {
		s.mu.Unlock()
		return
	}
	s.phase, s.source, s.samples, s.failures, s.errText, s.cancelled = PhaseIdle, "", nil, nil, "", false
	s.mu.Unlock()
	s.notify()
}

// ShowPrevious shows the remembered result.
func (s *Session) ShowPrevious() error {
	s.mu.Lock()
	if s.phase == PhaseRunning {
		s.mu.Unlock()
		return ErrBusy
	}
	if s.previous == nil {
		s.mu.Unlock()
		return ErrNoPrevious
	}
	p := *s.previous
	s.phase, s.source = PhaseDone, SourceArchive
	s.request = Request{Models: p.models(), Prompts: p.prompts(), Repeats: p.RepeatsOrMeasured()}
	s.sys, s.measuredAt, s.samples, s.failures = p.System, p.MeasuredAt, p.Samples, p.Failures
	s.cancelled, s.errText, s.step, s.live = p.Cancelled, "", nil, nil
	s.mu.Unlock()
	s.notify()
	return nil
}

// Shown returns the result on screen, if a comparison has finished or
// was loaded.
func (s *Session) Shown() (Result, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.phase != PhaseDone || len(s.samples) == 0 {
		return Result{}, false
	}
	return Result{MeasuredAt: s.measuredAt, System: s.sys, Repeats: s.request.Repeats, Cancelled: s.cancelled, Samples: s.samples, Failures: s.failures}, true
}

// View returns the current snapshot.
func (s *Session) View() View {
	s.mu.Lock()
	defer s.mu.Unlock()
	v := View{Phase: s.phase, Comparison: []bench.ModelComparison{}}
	if s.previous != nil {
		at := s.previous.MeasuredAt
		v.PreviousAt = &at
	}
	if s.phase == PhaseIdle {
		return v
	}
	at, sys := s.measuredAt, s.sys
	v.Source, v.MeasuredAt, v.System = s.source, &at, &sys
	v.Models, v.Prompts, v.Repeats = s.request.Models, s.request.Prompts, s.request.Repeats
	v.Step, v.Live = s.step, s.live
	v.Done, v.Total = len(s.samples), len(s.request.Models)*len(s.request.Prompts)*2*s.request.Repeats
	v.Cancelled, v.Error = s.cancelled, s.errText
	for _, f := range s.failures {
		v.Failures = append(v.Failures, Explain(f))
	}
	v.Comparison = bench.Compare(bench.Summarize(s.samples))
	return v
}

// Subscribe returns a channel that receives a signal after every change.
// Signals coalesce: a slow reader sees the latest state on its next View.
func (s *Session) Subscribe() (<-chan struct{}, func()) {
	ch := make(chan struct{}, 1)
	s.mu.Lock()
	s.subscribers[ch] = struct{}{}
	s.mu.Unlock()
	return ch, func() {
		s.mu.Lock()
		delete(s.subscribers, ch)
		s.mu.Unlock()
	}
}

func (s *Session) notify() {
	s.mu.Lock()
	defer s.mu.Unlock()
	for ch := range s.subscribers {
		select {
		case ch <- struct{}{}:
		default:
		}
	}
}

// observer feeds runner events into the session.
type observer struct{ s *Session }

func (o observer) Step(step bench.Step) {
	o.s.mu.Lock()
	o.s.step, o.s.live = &step, nil
	o.s.mu.Unlock()
	o.s.notify()
}

func (o observer) Live(l bench.Live) {
	o.s.mu.Lock()
	o.s.live = &l
	o.s.mu.Unlock()
	o.s.notify()
}

func (o observer) Failed(f bench.Failure) {
	o.s.mu.Lock()
	o.s.failures = append(o.s.failures, f)
	o.s.step, o.s.live = nil, nil
	o.s.mu.Unlock()
	o.s.notify()
}

func (o observer) Measured(sample bench.Sample) {
	o.s.mu.Lock()
	o.s.samples = append(o.s.samples, sample)
	o.s.live = nil
	o.s.mu.Unlock()
	o.s.notify()
}

func validate(req Request) error {
	if len(req.Models) == 0 {
		return fmt.Errorf("%w: kein Modell gewählt", ErrInvalid)
	}
	if len(req.Prompts) == 0 {
		return fmt.Errorf("%w: keine Aufgabe gewählt", ErrInvalid)
	}
	if req.Repeats < 1 || req.Repeats > MaxRepeats {
		return fmt.Errorf("%w: Durchläufe müssen zwischen 1 und %d liegen", ErrInvalid, MaxRepeats)
	}
	if dup := firstDuplicate(req.Models); dup != "" {
		return fmt.Errorf("%w: Modell %q doppelt", ErrInvalid, dup)
	}
	if dup := firstDuplicate(req.Prompts); dup != "" {
		return fmt.Errorf("%w: Aufgabe %q doppelt", ErrInvalid, dup)
	}
	for _, name := range req.Prompts {
		if _, ok := promptByName(name); !ok {
			return fmt.Errorf("%w: unbekannte Aufgabe %q", ErrInvalid, name)
		}
	}
	return nil
}

func planFor(req Request) bench.Plan {
	prompts := make([]bench.Prompt, 0, len(req.Prompts))
	for _, name := range req.Prompts {
		p, _ := promptByName(name)
		prompts = append(prompts, p)
	}
	return bench.Plan{
		Models:          req.Models,
		Engines:         []bench.Engine{bench.EngineOMLX, bench.EngineSplash},
		Prompts:         prompts,
		Repeats:         req.Repeats,
		MaxTokens:       MaxTokens,
		WarmupMaxTokens: WarmupMaxTokens,
	}
}

func promptByName(name string) (bench.Prompt, bool) {
	for _, p := range bench.DefaultPrompts {
		if p.Name == name {
			return p, true
		}
	}
	return bench.Prompt{}, false
}

func firstDuplicate(names []string) string {
	seen := map[string]bool{}
	for _, n := range names {
		if seen[n] {
			return n
		}
		seen[n] = true
	}
	return ""
}

func joinText(a, b string) string {
	if a == "" {
		return b
	}
	return a + " · " + b
}
