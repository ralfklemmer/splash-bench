package demo

import (
	"context"
	"errors"
	"math"
	"strings"
	"testing"
	"time"

	"github.com/ralfklemmer/splash-bench/internal/bench"
)

// fakeServer records engine changes and streams scripted replies.
type fakeServer struct {
	calls     []string
	engine    bench.Engine
	setErr    map[bench.Engine]error
	streamErr map[bench.Engine]error
	onStream  func()
	clock     *fakeClock
	deltas    []string
	usage     int
	// step is how much time passes per streamed chunk, per engine.
	step map[bench.Engine]time.Duration
}

func (s *fakeServer) CurrentEngine(context.Context, string) (bench.Engine, error) {
	return s.engine, nil
}

func (s *fakeServer) SetEngine(_ context.Context, _ string, e bench.Engine) error {
	s.calls = append(s.calls, "set "+EngineName(e))
	if err := s.setErr[e]; err != nil {
		return err
	}
	s.engine = e
	return nil
}

func (s *fakeServer) Unload(context.Context, string) error {
	s.calls = append(s.calls, "unload")
	return nil
}

func (s *fakeServer) StreamText(ctx context.Context, req bench.ChatRequest, onDelta func(bench.Delta)) (bench.Usage, error) {
	if req.MaxTokens < 100 {
		s.calls = append(s.calls, "warmup")
		return bench.Usage{CompletionTokens: req.MaxTokens}, nil
	}
	s.calls = append(s.calls, "measure "+EngineName(s.engine))
	if s.onStream != nil {
		s.onStream()
	}
	if err := s.streamErr[s.engine]; err != nil {
		return bench.Usage{}, err
	}
	s.clock.advance(50 * time.Millisecond) // prefill
	for _, d := range s.deltas {
		if ctx.Err() != nil {
			return bench.Usage{}, ctx.Err()
		}
		onDelta(bench.Delta{Content: d})
		s.clock.advance(s.step[s.engine])
	}
	return bench.Usage{PromptTokens: 10, CompletionTokens: s.usage}, nil
}

type fakeClock struct{ now time.Time }

func (c *fakeClock) Now() time.Time          { return c.now }
func (c *fakeClock) advance(d time.Duration) { c.now = c.now.Add(d) }

// recordingDisplay keeps what the audience would see.
type recordingDisplay struct {
	events   []string
	text     strings.Builder
	progress []Progress
	summary  *Summary
}

func (d *recordingDisplay) Phase(e bench.Engine, text string) {
	d.events = append(d.events, "phase "+EngineName(e))
}
func (d *recordingDisplay) StartRun(e bench.Engine) {
	d.events = append(d.events, "start "+EngineName(e))
}
func (d *recordingDisplay) Text(delta bench.Delta) { d.text.WriteString(delta.Content) }
func (d *recordingDisplay) Progress(p Progress)    { d.progress = append(d.progress, p) }
func (d *recordingDisplay) FinishRun(r RunResult) {
	d.events = append(d.events, "finish "+EngineName(r.Engine))
}
func (d *recordingDisplay) Notice(text string) { d.events = append(d.events, "notice "+text) }
func (d *recordingDisplay) Summary(s Summary)  { d.summary = &s }

func newFixture(original bench.Engine) (*fakeServer, *recordingDisplay, *Race) {
	clock := &fakeClock{now: time.Unix(0, 0)}
	deltas := make([]string, 101) // 101 chunks of 7 chars = 707 chars
	for i := range deltas {
		deltas[i] = "func x\n"
	}
	server := &fakeServer{
		engine: original, clock: clock, deltas: deltas, usage: 202,
		step: map[bench.Engine]time.Duration{bench.EngineOMLX: 20 * time.Millisecond, bench.EngineSplash: 10 * time.Millisecond},
	}
	display := &recordingDisplay{}
	return server, display, NewRace(server, server, clock, display)
}

var demoConfig = Config{Model: "Qwen3.8-27B-oQ4e-mtp", Prompt: "Code bitte", MaxTokens: 512, WarmupTokens: 16}

func TestRaceMeasuresBothEnginesAndRestores(t *testing.T) {
	server, display, race := newFixture(bench.EngineSplash)

	summary, err := race.Run(context.Background(), demoConfig)
	if err != nil {
		t.Fatal(err)
	}
	wantCalls := "set oMLX,unload,warmup,measure oMLX,set Splash,unload,warmup,measure Splash,set Splash,unload"
	if got := strings.Join(server.calls, ","); got != wantCalls {
		t.Errorf("calls = %s\nwant    %s", got, wantCalls)
	}
	if server.engine != bench.EngineSplash {
		t.Errorf("engine after demo = %q, want splash", server.engine)
	}
	if len(summary.Runs) != 2 || display.summary == nil {
		t.Fatalf("summary = %+v", summary)
	}
	// 201 tokens after the first one in 100 steps: oMLX 2 s, Splash 1 s.
	if got := summary.Runs[0].Sample.DecodeTokensPerSecond(); math.Abs(got-100.5) > 0.01 {
		t.Errorf("oMLX tok/s = %v, want 100.5", got)
	}
	if got := summary.Factor(); math.Abs(got-2) > 0.001 {
		t.Errorf("factor = %v, want 2", got)
	}
	if got := summary.Runs[0].Sample.TimeToFirstToken; got != 50*time.Millisecond {
		t.Errorf("TTFT = %v, want 50ms", got)
	}
	if !strings.HasPrefix(display.text.String(), "func x\n") {
		t.Errorf("text not streamed to display")
	}
}

func TestRaceRefreshesProgressAndCalibratesEstimate(t *testing.T) {
	_, display, race := newFixture(bench.EngineOMLX)
	if _, err := race.Run(context.Background(), demoConfig); err != nil {
		t.Fatal(err)
	}
	var splash []Progress
	for _, p := range display.progress {
		if p.Engine == bench.EngineSplash {
			splash = append(splash, p)
		}
	}
	// Splash streams for 1 s and refreshes every 100 ms.
	if len(splash) < 9 || len(splash) > 11 {
		t.Fatalf("got %d Splash progress updates, want about 10", len(splash))
	}
	// After calibration on the oMLX run (707 chars = 202 tokens), the live
	// estimate for Splash is exact: 202 tokens after the last chunk.
	last := splash[len(splash)-1]
	if last.Tokens < 190 || last.Tokens > 202 {
		t.Errorf("estimated tokens = %v, want close to 202", last.Tokens)
	}
	if last.TokensPerSecond < 180 || last.TokensPerSecond > 220 {
		t.Errorf("live tok/s = %v, want about 200", last.TokensPerSecond)
	}
}

func TestRaceRestoresEngineOnFailure(t *testing.T) {
	incompatible := errors.New("HTTP 400: Splash needs the draft")
	tests := []struct {
		name      string
		setup     func(*fakeServer)
		wantErr   string
		wantCalls string
	}{
		{
			name:      "splash rejected",
			setup:     func(s *fakeServer) { s.setErr = map[bench.Engine]error{bench.EngineSplash: incompatible} },
			wantErr:   "Splash needs the draft",
			wantCalls: "set oMLX,unload,warmup,measure oMLX,set Splash,set oMLX,unload",
		},
		{
			name: "stream fails",
			setup: func(s *fakeServer) {
				s.streamErr = map[bench.Engine]error{bench.EngineOMLX: errors.New("HTTP 409 unload is pending")}
			},
			wantErr:   "unload is pending",
			wantCalls: "set oMLX,unload,warmup,measure oMLX,set oMLX,unload",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server, display, race := newFixture(bench.EngineOMLX)
			tt.setup(server)
			_, err := race.Run(context.Background(), demoConfig)
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("err = %v, want %q", err, tt.wantErr)
			}
			if got := strings.Join(server.calls, ","); got != tt.wantCalls {
				t.Errorf("calls = %s\nwant    %s", got, tt.wantCalls)
			}
			if display.summary != nil {
				t.Error("summary shown despite failure")
			}
		})
	}
}

func TestRaceRestoresEngineOnCancel(t *testing.T) {
	server, display, race := newFixture(bench.EngineSplash)
	ctx, cancel := context.WithCancel(context.Background())
	server.onStream = func() {
		if server.engine == bench.EngineOMLX {
			cancel() // Ctrl-C during the oMLX run
		}
	}
	_, err := race.Run(ctx, demoConfig)
	if !errors.Is(err, context.Canceled) {
		t.Errorf("err = %v, want canceled", err)
	}
	if server.engine != bench.EngineSplash {
		t.Errorf("engine = %q, want restored splash", server.engine)
	}
	if !strings.Contains(strings.Join(display.events, "|"), "notice Abgebrochen.") {
		t.Errorf("events = %v, want abort notice", display.events)
	}
}

func TestRaceJoinsRestoreError(t *testing.T) {
	server, _, race := newFixture(bench.EngineSplash)
	server.streamErr = map[bench.Engine]error{bench.EngineOMLX: errors.New("boom")}
	server.setErr = map[bench.Engine]error{bench.EngineSplash: errors.New("restore failed")}
	_, err := race.Run(context.Background(), demoConfig)
	if err == nil || !strings.Contains(err.Error(), "boom") || !strings.Contains(err.Error(), "restore failed") {
		t.Errorf("err = %v, want both errors", err)
	}
}

func TestSummaryVerdict(t *testing.T) {
	run := func(e bench.Engine, rate float64) RunResult {
		return RunResult{Engine: e, Sample: bench.Sample{CompletionTokens: int(rate) + 1, DecodeDuration: time.Second}}
	}
	tests := []struct {
		runs []RunResult
		want string
	}{
		{[]RunResult{run(bench.EngineOMLX, 72), run(bench.EngineSplash, 114)}, "Splash generiert 1,58× so schnell wie oMLX."},
		{[]RunResult{run(bench.EngineOMLX, 224), run(bench.EngineSplash, 217)}, "Splash erreicht hier nur 0,97× der Geschwindigkeit von oMLX."},
		{[]RunResult{run(bench.EngineOMLX, 72)}, "Kein Vergleich möglich."},
	}
	for _, tt := range tests {
		if got := verdict(Summary{Runs: tt.runs}); got != tt.want {
			t.Errorf("verdict = %q, want %q", got, tt.want)
		}
	}
}

func TestWriteSummaryShowsBothEngines(t *testing.T) {
	s := Summary{Model: "m", Runs: []RunResult{
		{Engine: bench.EngineOMLX, Sample: bench.Sample{TimeToFirstToken: 146 * time.Millisecond, CompletionTokens: 146, DecodeDuration: 2 * time.Second}, Total: 2146 * time.Millisecond},
		{Engine: bench.EngineSplash, Sample: bench.Sample{TimeToFirstToken: 58 * time.Millisecond, CompletionTokens: 115, DecodeDuration: time.Second}, Total: 1058 * time.Millisecond},
	}}
	var b strings.Builder
	if err := WriteSummary(&b, s); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"Ergebnis m", "146 ms", "58 ms", "72,5 tok/s", "114,0 tok/s", "2,1 s", "1,57×"} {
		if !strings.Contains(b.String(), want) {
			t.Errorf("missing %q in:\n%s", want, b.String())
		}
	}
}

func TestTruncate(t *testing.T) {
	tests := []struct {
		s     string
		width int
		want  string
	}{
		{"kurz", 10, "kurz"},
		{"▶ Splash läuft", 8, "▶ Splas…"},
		{"abc", 0, "abc"},
	}
	for _, tt := range tests {
		if got := truncate(tt.s, tt.width); got != tt.want {
			t.Errorf("truncate(%q, %d) = %q, want %q", tt.s, tt.width, got, tt.want)
		}
	}
}

func TestTerminalDisplayRestoresTerminal(t *testing.T) {
	var b strings.Builder
	d := NewTerminalDisplay(&b, 30, 80, "Titel")
	d.Phase(bench.EngineOMLX, "laden")
	d.StartRun(bench.EngineOMLX)
	d.Text(bench.Delta{Content: "package main"})
	d.Progress(Progress{Engine: bench.EngineOMLX, Elapsed: time.Second, FirstToken: 100 * time.Millisecond, Tokens: 50, TokensPerSecond: 54.4})
	d.Close()
	out := b.String()
	for _, want := range []string{"\033[5;30r", "package main", "≈ 54,4 tok/s", escResetRegion + "\033[30;1H", escShowCursor} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in output %q", want, out)
		}
	}
	if !strings.HasSuffix(out, escShowCursor) {
		t.Error("cursor not shown again at the end")
	}
}
