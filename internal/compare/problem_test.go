package compare

import (
	"testing"

	"github.com/ralfklemmer/splash-bench/internal/bench"
)

func TestExplain(t *testing.T) {
	tied := "warm up Qwen3.5-35B-A3B-4bit/splash: POST /v1/chat/completions: oMLX antwortet mit HTTP 409: Model 'Qwen3.5-35B-A3B-4bit' failed to load: Splash exited with code 1 while loading Qwen3.5-35B-A3B-4bit: error: text config tie_word_embeddings must be false\n" +
		"error: model download or verification failed (log: /Users/x/.omlx/logs/splash/q.log). Reload models after fixing the files to retry."
	tests := []struct {
		name    string
		failure bench.Failure
		title   string
		detail  string
		log     string
	}{
		{"known cause", bench.Failure{Engine: bench.EngineSplash, Stage: bench.StepWarmup, Message: tied},
			"Splash konnte das Modell nicht laden", knownCauses[0].text, "/Users/x/.omlx/logs/splash/q.log"},
		{"unknown Splash error", bench.Failure{Engine: bench.EngineSplash, Stage: bench.StepWarmup, Message: "failed to load: x\nerror: unsupported architecture qwen9\nerror: model download or verification failed (log: /l.log)."},
			"Splash konnte das Modell nicht laden", "unsupported architecture qwen9", "/l.log"},
		{"check before measuring", bench.Failure{Engine: bench.EngineSplash, Stage: bench.StepCheck, Message: tied},
			"Splash konnte das Modell nicht laden", knownCauses[0].text, "/Users/x/.omlx/logs/splash/q.log"},
		{"switch", bench.Failure{Engine: bench.EngineSplash, Stage: bench.StepSwitch, Message: "set engine: refused"},
			"Splash ließ sich für dieses Modell nicht einschalten", "", ""},
		{"measurement", bench.Failure{Engine: bench.EngineOMLX, Stage: bench.StepMeasure, Message: "connection reset"},
			"Messung mit oMLX abgebrochen", "", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := Explain(tt.failure)
			if p.Title != tt.title || p.Detail != tt.detail || p.Log != tt.log || p.Raw != tt.failure.Message {
				t.Errorf("Explain = %+v\nwant title %q, detail %q, log %q", p, tt.title, tt.detail, tt.log)
			}
		})
	}
}
