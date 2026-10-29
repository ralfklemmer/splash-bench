package bench

import (
	"errors"
	"math"
	"strings"
	"testing"
	"time"
)

var t0 = time.Date(2026, 10, 17, 12, 0, 0, 0, time.UTC)

func TestNewSample(t *testing.T) {
	tests := []struct {
		name     string
		first    time.Time
		last     time.Time
		tokens   int
		wantErr  bool
		wantTTFT time.Duration
		wantRate float64
	}{
		{"normal run", t0.Add(500 * time.Millisecond), t0.Add(2500 * time.Millisecond), 101, false, 500 * time.Millisecond, 50},
		{"single token", t0.Add(time.Second), t0.Add(time.Second), 1, true, 0, 0},
		{"no chunk", time.Time{}, time.Time{}, 10, true, 0, 0},
		{"last not after first", t0.Add(time.Second), t0.Add(time.Second), 10, true, 0, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s, err := NewSample("m", EngineSplash, "p", t0, tt.first, tt.last, Usage{CompletionTokens: tt.tokens})
			if tt.wantErr {
				if !errors.Is(err, ErrTooFewTokens) {
					t.Fatalf("err = %v, want ErrTooFewTokens", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if s.TimeToFirstToken != tt.wantTTFT {
				t.Errorf("TTFT = %v, want %v", s.TimeToFirstToken, tt.wantTTFT)
			}
			if got := s.DecodeTokensPerSecond(); math.Abs(got-tt.wantRate) > 1e-9 {
				t.Errorf("rate = %v, want %v", got, tt.wantRate)
			}
		})
	}
}

func TestMedian(t *testing.T) {
	tests := []struct {
		values []float64
		want   float64
	}{
		{nil, 0},
		{[]float64{3}, 3},
		{[]float64{5, 1, 3}, 3},
		{[]float64{4, 1, 3, 2}, 2.5},
	}
	for _, tt := range tests {
		if got := median(tt.values); got != tt.want {
			t.Errorf("median(%v) = %v, want %v", tt.values, got, tt.want)
		}
	}
}

func TestSpeedup(t *testing.T) {
	tests := []struct {
		baseline, candidate, want float64
	}{
		{40, 100, 2.5},
		{50, 25, 0.5},
		{0, 100, 0},
	}
	for _, tt := range tests {
		if got := Speedup(tt.baseline, tt.candidate); got != tt.want {
			t.Errorf("Speedup(%v, %v) = %v, want %v", tt.baseline, tt.candidate, got, tt.want)
		}
	}
}

func sample(engine Engine, prompt string, ttft time.Duration, tokens int, decode time.Duration) Sample {
	return Sample{Model: "m", Engine: engine, Prompt: prompt, TimeToFirstToken: ttft, DecodeDuration: decode, CompletionTokens: tokens}
}

func TestSummarizeTakesMediansPerGroup(t *testing.T) {
	samples := []Sample{
		sample(EngineOMLX, "code", 100*time.Millisecond, 41, time.Second),   // 40 tok/s
		sample(EngineOMLX, "code", 300*time.Millisecond, 61, time.Second),   // 60 tok/s
		sample(EngineOMLX, "code", 200*time.Millisecond, 51, time.Second),   // 50 tok/s
		sample(EngineSplash, "code", 50*time.Millisecond, 101, time.Second), // 100 tok/s
	}
	got := Summarize(samples)
	if len(got) != 2 {
		t.Fatalf("got %d summaries, want 2", len(got))
	}
	if got[0].Engine != EngineOMLX || got[0].Runs != 3 || got[0].DecodeTokensPerSecond != 50 || got[0].TimeToFirstToken != 200*time.Millisecond {
		t.Errorf("oMLX summary = %+v", got[0])
	}
	if got[1].Engine != EngineSplash || got[1].DecodeTokensPerSecond != 100 {
		t.Errorf("Splash summary = %+v", got[1])
	}
}

func TestWriteMarkdownShowsSpeedupOnlyForSplash(t *testing.T) {
	var out strings.Builder
	err := WriteMarkdown(&out, []Summary{
		{Model: "m", Engine: EngineOMLX, Prompt: "code", Runs: 1, DecodeTokensPerSecond: 40},
		{Model: "m", Engine: EngineSplash, Prompt: "code", Runs: 1, DecodeTokensPerSecond: 100},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "| m | splash | code | 1 | 0s | 100.0 | 2.50× |") {
		t.Errorf("missing Splash row with speedup:\n%s", out.String())
	}
	if !strings.Contains(out.String(), "| m | oMLX | code | 1 | 0s | 40.0 | – |") {
		t.Errorf("missing oMLX row:\n%s", out.String())
	}
}
