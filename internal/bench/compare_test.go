package bench

import (
	"math"
	"testing"
	"time"
)

func summary(model string, engine Engine, prompt string, tps float64) Summary {
	return Summary{Model: model, Engine: engine, Prompt: prompt, Runs: 3, DecodeTokensPerSecond: tps, TimeToFirstToken: 50 * time.Millisecond}
}

// pairs builds oMLX and Splash summaries for one model: prompt → {oMLX, Splash}.
func pairs(model string, rates map[string][2]float64, order ...string) []Summary {
	var out []Summary
	for _, p := range order {
		r := rates[p]
		if r[0] > 0 {
			out = append(out, summary(model, EngineOMLX, p, r[0]))
		}
		if r[1] > 0 {
			out = append(out, summary(model, EngineSplash, p, r[1]))
		}
	}
	return out
}

func TestCompareVerdict(t *testing.T) {
	tests := []struct {
		name     string
		rates    map[string][2]float64
		order    []string
		wantMean float64
		want     Verdict
	}{
		{"27B from the brief", map[string][2]float64{"code": {72.5, 114.4}, "prose": {53.0, 73.3}, "translate": {69.2, 119.6}},
			[]string{"code", "prose", "translate"}, 1.556, VerdictSplash},
		{"35B-A3B from the brief", map[string][2]float64{"code": {224.3, 217.8}, "prose": {196.4, 140.2}, "translate": {208.4, 268.0}},
			[]string{"code", "prose", "translate"}, 0.962, VerdictOMLX},
		{"half worthwhile is not enough", map[string][2]float64{"a": {100, 150}, "b": {100, 100}},
			[]string{"a", "b"}, 1.225, VerdictOMLX},
		{"exactly the threshold counts", map[string][2]float64{"a": {100, 115}},
			[]string{"a"}, 1.15, VerdictSplash},
		{"splash not measured yet", map[string][2]float64{"a": {100, 0}},
			[]string{"a"}, 0, VerdictOpen},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Compare(pairs("m", tt.rates, tt.order...))
			if len(got) != 1 {
				t.Fatalf("got %d models, want 1", len(got))
			}
			if got[0].Verdict != tt.want {
				t.Errorf("verdict = %q, want %q", got[0].Verdict, tt.want)
			}
			if math.Abs(got[0].MeanSpeedup-tt.wantMean) > 0.001 {
				t.Errorf("mean speedup = %.4f, want %.3f", got[0].MeanSpeedup, tt.wantMean)
			}
		})
	}
}

func TestCompareKeepsOrderAndPairsEngines(t *testing.T) {
	summaries := []Summary{
		summary("b", EngineOMLX, "code", 50),
		summary("a", EngineOMLX, "prose", 40),
		summary("b", EngineSplash, "code", 75),
		summary("b", EngineOMLX, "prose", 30),
	}
	got := Compare(summaries)
	if len(got) != 2 || got[0].Model != "b" || got[1].Model != "a" {
		t.Fatalf("models = %+v, want b then a", got)
	}
	b := got[0]
	if len(b.Prompts) != 2 || b.Prompts[0].Prompt != "code" || b.Prompts[1].Prompt != "prose" {
		t.Fatalf("prompts of b = %+v", b.Prompts)
	}
	if b.Prompts[0].Speedup != 1.5 || b.Prompts[1].Speedup != 0 || b.Prompts[1].Splash != nil {
		t.Errorf("prompts of b = %+v", b.Prompts)
	}
}
