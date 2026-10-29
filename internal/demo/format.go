package demo

import (
	"fmt"
	"io"
	"strings"
	"text/tabwriter"
	"time"
)

// decimal formats f with prec decimals and a German decimal comma.
func decimal(f float64, prec int) string {
	return strings.Replace(fmt.Sprintf("%.*f", prec, f), ".", ",", 1)
}

func millis(d time.Duration) string {
	return fmt.Sprintf("%d ms", d.Milliseconds())
}

func seconds(d time.Duration) string {
	return decimal(d.Seconds(), 1) + " s"
}

// resultLine is the one-line result of a finished run.
func resultLine(r RunResult) string {
	return fmt.Sprintf("%s: %s tok/s · erstes Token %s · %d Tokens in %s",
		EngineName(r.Engine), decimal(r.Sample.DecodeTokensPerSecond(), 1),
		millis(r.Sample.TimeToFirstToken), r.Sample.CompletionTokens, seconds(r.Total))
}

// progressLine is the live status of a running measurement.
func progressLine(p Progress) string {
	name := EngineName(p.Engine)
	if p.FirstToken == 0 {
		return fmt.Sprintf("▶ %s  %s  warte auf erstes Token …", name, seconds(p.Elapsed))
	}
	return fmt.Sprintf("▶ %s  %s  erstes Token %s  ≈ %.0f Tokens  ≈ %s tok/s",
		name, seconds(p.Elapsed), millis(p.FirstToken), p.Tokens, decimal(p.TokensPerSecond, 1))
}

// WriteSummary prints the side-by-side comparison of a race.
func WriteSummary(w io.Writer, s Summary) error {
	var b strings.Builder
	fmt.Fprintf(&b, "Ergebnis %s\n\n", s.Model)
	tw := tabwriter.NewWriter(&b, 0, 0, 3, ' ', 0)
	row := func(label string, value func(RunResult) string) {
		cells := []string{"  " + label}
		for _, r := range s.Runs {
			cells = append(cells, value(r))
		}
		fmt.Fprintln(tw, strings.Join(cells, "\t"))
	}
	row("", func(r RunResult) string { return EngineName(r.Engine) })
	row("Erstes Token", func(r RunResult) string { return millis(r.Sample.TimeToFirstToken) })
	row("Generierung", func(r RunResult) string { return decimal(r.Sample.DecodeTokensPerSecond(), 1) + " tok/s" })
	row("Tokens", func(r RunResult) string { return fmt.Sprint(r.Sample.CompletionTokens) })
	row("Dauer", func(r RunResult) string { return seconds(r.Total) })
	if err := tw.Flush(); err != nil {
		return err
	}
	b.WriteString("\n  " + verdict(s) + "\n")
	_, err := io.WriteString(w, b.String())
	return err
}

func verdict(s Summary) string {
	f := s.Factor()
	if len(s.Runs) < 2 || f == 0 {
		return "Kein Vergleich möglich."
	}
	first, last := EngineName(s.Runs[0].Engine), EngineName(s.Runs[len(s.Runs)-1].Engine)
	if f >= 1 {
		return fmt.Sprintf("%s generiert %s× so schnell wie %s.", last, decimal(f, 2), first)
	}
	return fmt.Sprintf("%s erreicht hier nur %s× der Geschwindigkeit von %s.", last, decimal(f, 2), first)
}
