// Package compare runs one oMLX-versus-Splash comparison at a time for the
// graphical app: it validates the request, follows the benchmark live and
// keeps the finished result.
package compare

import (
	"slices"
	"time"

	"github.com/ralfklemmer/splash-bench/internal/bench"
	"github.com/ralfklemmer/splash-bench/internal/check"
)

// Result is one finished (or cancelled) comparison as it is stored.
type Result struct {
	MeasuredAt time.Time      `json:"measured_at"`
	System     check.System   `json:"system"`
	Repeats    int            `json:"repeats"`
	Cancelled  bool           `json:"cancelled,omitempty"`
	Samples    []bench.Sample `json:"samples"`
	// Failures are the models that could not be measured completely.
	Failures []bench.Failure `json:"failures,omitempty"`
}

// Comparison sums up the samples per model and prompt.
func (r Result) Comparison() []bench.ModelComparison {
	return bench.Compare(bench.Summarize(r.Samples))
}

// RepeatsOrMeasured is Repeats, or for results without it (files written
// by splashbench) the largest number of runs of any model, engine and prompt.
func (r Result) RepeatsOrMeasured() int {
	if r.Repeats > 0 {
		return r.Repeats
	}
	most := 0
	for _, s := range bench.Summarize(r.Samples) {
		most = max(most, s.Runs)
	}
	return most
}

// models and prompts list the names in order of first appearance.
func (r Result) models() []string {
	models := distinct(r.Samples, func(s bench.Sample) string { return s.Model })
	for _, f := range r.Failures {
		if !slices.Contains(models, f.Model) {
			models = append(models, f.Model)
		}
	}
	return models
}

func (r Result) prompts() []string {
	return distinct(r.Samples, func(s bench.Sample) string { return s.Prompt })
}

func distinct(samples []bench.Sample, key func(bench.Sample) string) []string {
	var out []string
	seen := map[string]bool{}
	for _, s := range samples {
		k := key(s)
		if !seen[k] {
			seen[k] = true
			out = append(out, k)
		}
	}
	return out
}
