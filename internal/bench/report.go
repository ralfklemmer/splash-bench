package bench

import (
	"fmt"
	"io"
	"time"
)

// Summary is the median result of one model, engine and prompt.
type Summary struct {
	Model                 string
	Engine                Engine
	Prompt                string
	Runs                  int
	TimeToFirstToken      time.Duration
	DecodeTokensPerSecond float64
}

type summaryKey struct {
	model  string
	engine Engine
	prompt string
}

// Summarize groups samples by model, engine and prompt and takes medians.
// The order follows the first appearance of each group in samples.
func Summarize(samples []Sample) []Summary {
	var order []summaryKey
	ttfts := map[summaryKey][]float64{}
	rates := map[summaryKey][]float64{}
	for _, s := range samples {
		key := summaryKey{s.Model, s.Engine, s.Prompt}
		if _, seen := rates[key]; !seen {
			order = append(order, key)
		}
		ttfts[key] = append(ttfts[key], float64(s.TimeToFirstToken))
		rates[key] = append(rates[key], s.DecodeTokensPerSecond())
	}
	summaries := make([]Summary, 0, len(order))
	for _, key := range order {
		summaries = append(summaries, Summary{
			Model:                 key.model,
			Engine:                key.engine,
			Prompt:                key.prompt,
			Runs:                  len(rates[key]),
			TimeToFirstToken:      time.Duration(median(ttfts[key])),
			DecodeTokensPerSecond: median(rates[key]),
		})
	}
	return summaries
}

// Speedup is the decode rate of candidate relative to baseline, or 0 when
// the baseline has no rate.
func Speedup(baseline, candidate float64) float64 {
	if baseline <= 0 {
		return 0
	}
	return candidate / baseline
}

// WriteMarkdown writes one table row per summary and, where both engines
// were measured, the Splash speedup over oMLX.
func WriteMarkdown(w io.Writer, summaries []Summary) error {
	baseline := map[summaryKey]float64{}
	for _, s := range summaries {
		if s.Engine == EngineOMLX {
			baseline[summaryKey{s.Model, EngineOMLX, s.Prompt}] = s.DecodeTokensPerSecond
		}
	}
	header := "| Modell | Engine | Prompt | Läufe | TTFT (Median) | tok/s (Median) | Faktor vs. oMLX |\n" +
		"|---|---|---|---|---|---|---|\n"
	if _, err := io.WriteString(w, header); err != nil {
		return fmt.Errorf("write report header: %w", err)
	}
	for _, s := range summaries {
		factor := "–"
		if s.Engine != EngineOMLX {
			if f := Speedup(baseline[summaryKey{s.Model, EngineOMLX, s.Prompt}], s.DecodeTokensPerSecond); f > 0 {
				factor = fmt.Sprintf("%.2f×", f)
			}
		}
		if _, err := fmt.Fprintf(w, "| %s | %s | %s | %d | %v | %.1f | %s |\n",
			s.Model, s.Engine.Label(), s.Prompt, s.Runs,
			s.TimeToFirstToken.Round(time.Millisecond), s.DecodeTokensPerSecond, factor); err != nil {
			return fmt.Errorf("write report row: %w", err)
		}
	}
	return nil
}
