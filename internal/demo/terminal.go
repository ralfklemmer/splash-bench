package demo

import (
	"fmt"
	"io"
	"strings"

	"github.com/ralfklemmer/splash-bench/internal/bench"
)

// TerminalDisplay shows the race full-screen: a fixed header with the live
// status at the top and the streamed text scrolling below it. It uses
// plain ANSI/VT100 sequences that Terminal.app, iTerm2 and Ghostty support.
type TerminalDisplay struct {
	w          io.Writer
	rows, cols int
	title      string
	engine     *bench.Engine
	previous   []RunResult
	midLine    bool
}

// Header rows: title, status, previous result, separator.
const headerRows = 4

const (
	escClearScreen = "\033[2J\033[H"
	escClearLine   = "\033[2K"
	escSaveCursor  = "\0337"
	escLoadCursor  = "\0338"
	escHideCursor  = "\033[?25l"
	escShowCursor  = "\033[?25h"
	escResetRegion = "\033[r"
	escReset       = "\033[0m"
	escBold        = "\033[1m"
	escDim         = "\033[2m"
	escInverse     = "\033[7m"
	escGreen       = "\033[32m"
)

// engineColor makes the two engines easy to tell apart on screen.
func engineColor(e bench.Engine) string {
	if e == bench.EngineSplash {
		return "\033[1;36m" // bold cyan
	}
	return "\033[1;33m" // bold yellow
}

func NewTerminalDisplay(w io.Writer, rows, cols int, title string) *TerminalDisplay {
	return &TerminalDisplay{w: w, rows: rows, cols: cols, title: title}
}

func (d *TerminalDisplay) Phase(engine bench.Engine, text string) {
	if d.engine == nil || *d.engine != engine {
		d.drawScreen(engine)
	}
	d.status(engineColor(engine), "⟳ "+EngineName(engine)+": "+text)
}

func (d *TerminalDisplay) StartRun(engine bench.Engine) {
	d.status(engineColor(engine), "▶ "+EngineName(engine)+": Prompt gesendet …")
}

func (d *TerminalDisplay) Text(delta bench.Delta) {
	if delta.Reasoning != "" {
		d.write(escDim + delta.Reasoning + escReset)
	}
	d.write(delta.Content)
	text := delta.Reasoning + delta.Content
	if text != "" {
		d.midLine = !strings.HasSuffix(text, "\n")
	}
}

func (d *TerminalDisplay) Progress(p Progress) {
	d.status(engineColor(p.Engine), progressLine(p))
}

func (d *TerminalDisplay) FinishRun(r RunResult) {
	d.status(escGreen+escBold, "✓ "+resultLine(r))
	d.previous = append(d.previous, r)
}

func (d *TerminalDisplay) Notice(text string) {
	d.leaveFullScreen()
	d.write(text + "\n")
}

func (d *TerminalDisplay) Summary(s Summary) {
	d.leaveFullScreen()
	var b strings.Builder
	WriteSummary(&b, s)
	d.write(escBold + b.String() + escReset)
}

// Close restores the terminal; it is safe to call more than once.
func (d *TerminalDisplay) Close() {
	d.leaveFullScreen()
	d.write(escShowCursor)
}

func (d *TerminalDisplay) drawScreen(engine bench.Engine) {
	d.engine = &engine
	d.midLine = false
	d.write(escResetRegion + escHideCursor + escClearScreen)
	d.line(1, escInverse+escBold, " "+d.title)
	d.line(3, escDim, d.previousLine())
	d.line(4, escDim, strings.Repeat("─", d.cols))
	d.write(fmt.Sprintf("\033[%d;%dr\033[%d;1H", headerRows+1, d.rows, headerRows+1))
}

func (d *TerminalDisplay) previousLine() string {
	parts := make([]string, 0, len(d.previous))
	for _, r := range d.previous {
		parts = append(parts, "zuvor "+resultLine(r))
	}
	return strings.Join(parts, "   ")
}

// status rewrites the status row without moving the text cursor.
func (d *TerminalDisplay) status(style, text string) {
	if d.engine == nil {
		return
	}
	d.write(escSaveCursor)
	d.line(2, style, text)
	d.write(escLoadCursor)
}

func (d *TerminalDisplay) line(row int, style, text string) {
	d.write(fmt.Sprintf("\033[%d;1H%s%s%s%s", row, escClearLine, style, truncate(text, d.cols), escReset))
}

// leaveFullScreen drops the scroll region and puts the cursor below the text.
func (d *TerminalDisplay) leaveFullScreen() {
	if d.engine == nil {
		return
	}
	d.engine = nil
	d.write(escResetRegion + fmt.Sprintf("\033[%d;1H", d.rows))
	if d.midLine {
		d.write("\n")
	}
	d.write("\n")
	d.midLine = false
}

func (d *TerminalDisplay) write(s string) {
	io.WriteString(d.w, s)
}

func truncate(s string, width int) string {
	runes := []rune(s)
	if width <= 0 || len(runes) <= width {
		return s
	}
	if width == 1 {
		return "…"
	}
	return string(runes[:width-1]) + "…"
}
