package compare

import (
	"regexp"
	"strings"

	"github.com/ralfklemmer/splash-bench/internal/bench"
)

// Problem explains a bench.Failure in plain German for the app.
type Problem struct {
	Model  string         `json:"model"`
	Engine bench.Engine   `json:"engine"`
	Stage  bench.StepKind `json:"stage"`
	Title  string         `json:"title"`
	Detail string         `json:"detail"`
	Log    string         `json:"log,omitempty"`
	Raw    string         `json:"raw"`
}

// knownCauses translates Splash's load errors the app has seen.
var knownCauses = []struct{ match, text string }{
	{"tie_word_embeddings must be false",
		"Das Modell teilt sich Ein- und Ausgabe-Gewichte (tie_word_embeddings). Splash unterstützt solche Modelle nicht – oMLX prüft das vorab nicht und meldet sie trotzdem als Splash-fähig."},
}

var (
	logPattern   = regexp.MustCompile(`\(log: ([^)]+)\)`)
	errorPattern = regexp.MustCompile(`(?m)^error: (.+)$`)
)

// genericErrors are follow-up lines that say nothing about the cause.
var genericErrors = []string{"model download or verification failed"}

// Explain turns a failure into a title, the likely cause and the log file
// to look into. Raw keeps the original message for bug reports.
func Explain(f bench.Failure) Problem {
	p := Problem{Model: f.Model, Engine: f.Engine, Stage: f.Stage, Raw: f.Message, Title: title(f)}
	if m := logPattern.FindStringSubmatch(f.Message); m != nil {
		p.Log = m[1]
	}
	p.Detail = cause(f.Message)
	return p
}

func title(f bench.Failure) string {
	engine := f.Engine.Label()
	if f.Engine == bench.EngineSplash {
		engine = "Splash"
	}
	switch f.Stage {
	case bench.StepSwitch:
		return engine + " ließ sich für dieses Modell nicht einschalten"
	case bench.StepCheck, bench.StepWarmup:
		return engine + " konnte das Modell nicht laden"
	default:
		return "Messung mit " + engine + " abgebrochen"
	}
}

func cause(message string) string {
	for _, k := range knownCauses {
		if strings.Contains(message, k.match) {
			return k.text
		}
	}
	for _, m := range errorPattern.FindAllStringSubmatch(message, -1) {
		line := strings.TrimSpace(logPattern.ReplaceAllString(m[1], ""))
		if !isGeneric(line) {
			return line
		}
	}
	return ""
}

func isGeneric(line string) bool {
	for _, g := range genericErrors {
		if strings.Contains(line, g) {
			return true
		}
	}
	return false
}
