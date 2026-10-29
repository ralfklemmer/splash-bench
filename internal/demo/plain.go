package demo

import (
	"fmt"
	"io"

	"github.com/ralfklemmer/splash-bench/internal/bench"
)

// PlainDisplay writes the race as plain lines, for pipes and log files.
type PlainDisplay struct {
	w io.Writer
}

func NewPlainDisplay(w io.Writer) *PlainDisplay {
	return &PlainDisplay{w: w}
}

func (d *PlainDisplay) Phase(engine bench.Engine, text string) {
	fmt.Fprintf(d.w, "[%s] %s\n", EngineName(engine), text)
}

func (d *PlainDisplay) StartRun(engine bench.Engine) {
	fmt.Fprintf(d.w, "[%s] Messung läuft\n\n", EngineName(engine))
}

func (d *PlainDisplay) Text(delta bench.Delta) {
	io.WriteString(d.w, delta.Reasoning+delta.Content)
}

func (d *PlainDisplay) Progress(Progress) {}

func (d *PlainDisplay) FinishRun(r RunResult) {
	fmt.Fprintf(d.w, "\n\n[%s] fertig – %s\n\n", EngineName(r.Engine), resultLine(r))
}

func (d *PlainDisplay) Notice(text string) {
	fmt.Fprintln(d.w, text)
}

func (d *PlainDisplay) Summary(s Summary) {
	WriteSummary(d.w, s)
}

// Close has nothing to clean up.
func (d *PlainDisplay) Close() {}
