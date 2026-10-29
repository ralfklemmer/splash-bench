package bench

import (
	"math"
	"time"
)

// WorthwhileSpeedup is the factor from which Splash counts as worth it for
// a prompt.
const WorthwhileSpeedup = 1.15

// Verdict is the recommendation for one model.
type Verdict string

const (
	VerdictOpen   Verdict = "open" // no prompt measured on both engines
	VerdictSplash Verdict = "splash"
	VerdictOMLX   Verdict = "omlx"
)

// EngineResult is the median result of one engine for one prompt.
type EngineResult struct {
	Runs             int           `json:"runs"`
	TokensPerSecond  float64       `json:"tokens_per_second"`
	TimeToFirstToken time.Duration `json:"time_to_first_token_ns"`
}

// PromptComparison sets both engines side by side for one prompt.
// Speedup is 0 until both engines are measured.
type PromptComparison struct {
	Prompt  string        `json:"prompt"`
	OMLX    *EngineResult `json:"omlx,omitempty"`
	Splash  *EngineResult `json:"splash,omitempty"`
	Speedup float64       `json:"speedup"`
}

// Worthwhile reports whether Splash is fast enough to switch for this prompt.
func (p PromptComparison) Worthwhile() bool {
	return p.Speedup >= WorthwhileSpeedup
}

// ModelComparison sums up one model. MeanSpeedup is the geometric mean over
// the prompts measured on both engines, 0 when there are none.
type ModelComparison struct {
	Model       string             `json:"model"`
	Prompts     []PromptComparison `json:"prompts"`
	MeanSpeedup float64            `json:"mean_speedup"`
	Verdict     Verdict            `json:"verdict"`
}

// Compare groups summaries by model and prompt in order of first
// appearance. Splash is recommended when it is worthwhile for more than
// half of the prompts measured on both engines.
func Compare(summaries []Summary) []ModelComparison {
	var models []*ModelComparison
	byModel := map[string]*ModelComparison{}
	for _, s := range summaries {
		mc, ok := byModel[s.Model]
		if !ok {
			mc = &ModelComparison{Model: s.Model}
			byModel[s.Model] = mc
			models = append(models, mc)
		}
		pc := promptOf(mc, s.Prompt)
		result := &EngineResult{Runs: s.Runs, TokensPerSecond: s.DecodeTokensPerSecond, TimeToFirstToken: s.TimeToFirstToken}
		if s.Engine == EngineOMLX {
			pc.OMLX = result
		} else {
			pc.Splash = result
		}
	}
	out := make([]ModelComparison, 0, len(models))
	for _, mc := range models {
		mc.conclude()
		out = append(out, *mc)
	}
	return out
}

func promptOf(mc *ModelComparison, prompt string) *PromptComparison {
	for i := range mc.Prompts {
		if mc.Prompts[i].Prompt == prompt {
			return &mc.Prompts[i]
		}
	}
	mc.Prompts = append(mc.Prompts, PromptComparison{Prompt: prompt})
	return &mc.Prompts[len(mc.Prompts)-1]
}

func (mc *ModelComparison) conclude() {
	var logSum float64
	measured, worthwhile := 0, 0
	for i := range mc.Prompts {
		p := &mc.Prompts[i]
		if p.OMLX == nil || p.Splash == nil {
			continue
		}
		p.Speedup = Speedup(p.OMLX.TokensPerSecond, p.Splash.TokensPerSecond)
		if p.Speedup <= 0 {
			continue
		}
		measured++
		logSum += math.Log(p.Speedup)
		if p.Worthwhile() {
			worthwhile++
		}
	}
	switch {
	case measured == 0:
		mc.Verdict = VerdictOpen
	case 2*worthwhile > measured:
		mc.Verdict = VerdictSplash
	default:
		mc.Verdict = VerdictOMLX
	}
	if measured > 0 {
		mc.MeanSpeedup = math.Exp(logSum / float64(measured))
	}
}
