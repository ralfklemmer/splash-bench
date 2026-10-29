package store

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/ralfklemmer/splash-bench/internal/bench"
	"github.com/ralfklemmer/splash-bench/internal/check"
	"github.com/ralfklemmer/splash-bench/internal/compare"
)

var t0 = time.Date(2026, 10, 17, 12, 0, 0, 0, time.Local)

func result(at time.Time, model string) compare.Result {
	return compare.Result{MeasuredAt: at, System: check.System{Chip: "M5 Max"}, Repeats: 3,
		Samples: []bench.Sample{{Model: model, Engine: bench.EngineSplash, Prompt: "code", CompletionTokens: 10}}}
}

func TestLatestWithoutDirectory(t *testing.T) {
	_, ok, err := NewDir(filepath.Join(t.TempDir(), "missing")).Latest()
	if ok || err != nil {
		t.Errorf("Latest = ok %v, err %v; want nothing and no error", ok, err)
	}
}

func TestSaveAndLatest(t *testing.T) {
	d := NewDir(filepath.Join(t.TempDir(), "results"))
	for _, r := range []compare.Result{result(t0.Add(time.Hour), "newer"), result(t0, "older")} {
		if err := d.Save(r); err != nil {
			t.Fatalf("Save: %v", err)
		}
	}

	got, ok, err := d.Latest()
	if err != nil || !ok {
		t.Fatalf("Latest: ok %v, err %v", ok, err)
	}
	if got.Samples[0].Model != "newer" || got.System.Chip != "M5 Max" || got.Repeats != 3 || !got.MeasuredAt.Equal(t0.Add(time.Hour)) {
		t.Errorf("Latest = %+v", got)
	}
	files, _ := filepath.Glob(filepath.Join(d.path, "*"))
	if len(files) != 2 {
		t.Errorf("files = %v, want exactly the two results (no temp files)", files)
	}
}

func TestReadFileAcceptsSplashbenchSamples(t *testing.T) {
	path := filepath.Join(t.TempDir(), "results.json")
	data := `[{"model":"m","engine":"","prompt":"code","time_to_first_token_ns":47255625,"decode_duration_ns":2400538167,"prompt_tokens":74,"completion_tokens":512}]`
	if err := os.WriteFile(path, []byte(data), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(path, t0, t0); err != nil {
		t.Fatal(err)
	}

	got, err := ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if len(got.Samples) != 1 || got.Samples[0].CompletionTokens != 512 || got.Samples[0].Engine != bench.EngineOMLX || !got.MeasuredAt.Equal(t0) {
		t.Errorf("ReadFile = %+v", got)
	}
}

func TestReadFileRejectsGarbage(t *testing.T) {
	path := filepath.Join(t.TempDir(), "x.json")
	if err := os.WriteFile(path, []byte("{nope"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := ReadFile(path); err == nil {
		t.Error("want a decode error")
	}
}
