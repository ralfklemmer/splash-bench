package readiness

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/ralfklemmer/splash-bench/internal/check"
)

type fakeServer struct {
	healthErr error
	modelsErr error
	models    []ServerModel
}

func (f fakeServer) Health(context.Context) error { return f.healthErr }
func (f fakeServer) Models(context.Context) ([]ServerModel, error) {
	return f.models, f.modelsErr
}

type fixedSystem check.System

func (s fixedSystem) System(context.Context) check.System { return check.System(s) }

func m5Max() check.System {
	return check.System{Chip: "M5 Max", AppleSilicon: true, MemoryGB: 128, MacOS: "27.0.1", Launcher: "splash", Splash: "1.5.0", OMLX: "0.7.1rc1"}
}

func splash(compatible bool, reason string) *SplashSupport {
	return &SplashSupport{Compatible: compatible, Reason: reason}
}

func missingDraft(repo string) *SplashSupport {
	return &SplashSupport{Reason: "Splash needs the DFlash2 draft", DraftRepo: repo}
}

func videoModels() []ServerModel {
	return []ServerModel{
		{ID: "Qwen3.8-27B-oQ4e-mtp", Splash: splash(true, "")},
		{ID: "Qwen3.6-35B-A3B-oQ4e-mtp", Splash: splash(true, "")},
		{ID: "gemma-4", Splash: splash(false, "Splash serves Qwen only")},
	}
}

func run(t *testing.T, server Server, sys check.System, keySet bool) Status {
	t.Helper()
	cfg := Config{BaseURL: "http://localhost:8000", BaseURLSource: "~/.omlx/settings.json", KeySet: keySet}
	return NewChecker(server, fixedSystem(sys), cfg).Status(context.Background())
}

// levels lists the level of every check in order.
func levels(st Status) string {
	var out []string
	for _, c := range st.Checks {
		out = append(out, string(c.Level))
	}
	return strings.Join(out, " ")
}

func find(t *testing.T, st Status, title string) Check {
	t.Helper()
	for _, c := range st.Checks {
		if c.Title == title {
			return c
		}
	}
	t.Fatalf("no check %q in %+v", title, st.Checks)
	return Check{}
}

func TestStatusLevels(t *testing.T) {
	intel := check.System{Chip: "Intel(R) Core(TM) i9", Launcher: "splash"}
	noSplash := m5Max()
	noSplash.Splash = ""
	oldSplash := m5Max()
	oldSplash.Splash = "1.2.9"
	noFlashNext := m5Max()
	noFlashNext.Splash = "1.3.2"
	oldMacOS := m5Max()
	oldMacOS.MacOS = "26.3"
	m2Port := check.System{Chip: "M2 Pro", AppleSilicon: true, MemoryGB: 32, MacOS: "26.4", Launcher: "splash-m1", Splash: "1.4.0"}
	m2NoPort := m2Port
	m2NoPort.Splash = ""
	m2OldMacOS := m2Port
	m2OldMacOS.MacOS = "15.7"
	small := m5Max()
	small.MemoryGB = 16 // 12 GB usable: neither model fits

	// Order: chip, macOS, Splash, reachable, access, support, model, memory.
	tests := []struct {
		name   string
		server fakeServer
		sys    check.System
		keySet bool
		want   string
		ready  bool
	}{
		{"everything fine", fakeServer{models: videoModels()}, m5Max(), true, "ok ok ok ok ok ok ok ok", true},
		{"no key, oMLX allows it", fakeServer{models: videoModels()}, m5Max(), false, "ok ok ok ok ok ok ok ok", true},
		{"Intel Mac, RAM and macOS unreadable", fakeServer{models: videoModels()}, intel, true, "fail warn fail ok ok ok ok warn", false},
		{"Splash missing", fakeServer{models: videoModels()}, noSplash, true, "ok ok fail ok ok ok ok ok", false},
		{"Splash too old", fakeServer{models: videoModels()}, oldSplash, true, "ok ok fail ok ok ok ok ok", false},
		{"Splash without Flash-Next", fakeServer{models: videoModels()}, noFlashNext, true, "ok ok warn ok ok ok ok ok", true},
		{"macOS too old", fakeServer{models: videoModels()}, oldMacOS, true, "ok fail ok ok ok ok ok ok", false},
		{"M2 with community port", fakeServer{models: videoModels()}, m2Port, true, "warn ok ok ok ok ok ok warn", true},
		{"M2 without community port", fakeServer{models: videoModels()}, m2NoPort, true, "warn ok fail ok ok ok ok warn", false},
		{"M2, old macOS", fakeServer{models: videoModels()}, m2OldMacOS, true, "warn warn ok ok ok ok ok warn", true},
		{"oMLX down", fakeServer{healthErr: errors.New("refused")}, m5Max(), true, "ok ok ok fail skip skip skip skip", false},
		{"oMLX starting", fakeServer{healthErr: ErrStarting}, m5Max(), true, "ok ok ok fail skip skip skip skip", false},
		{"key rejected", fakeServer{modelsErr: ErrUnauthorized}, m5Max(), true, "ok ok ok ok fail skip skip skip", false},
		{"oMLX without Splash support", fakeServer{models: []ServerModel{{ID: "Qwen3.8-27B-oQ4e-mtp"}}}, m5Max(), true, "ok ok ok ok ok fail skip skip", false},
		{"no Splash model", fakeServer{models: []ServerModel{{ID: "gemma-4", Splash: splash(false, "Qwen only")}}}, m5Max(), true, "ok ok ok ok ok ok fail skip", false},
		{"too little RAM", fakeServer{models: videoModels()}, small, true, "ok ok ok ok ok ok ok fail", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			st := run(t, tt.server, tt.sys, tt.keySet)
			if got := levels(st); got != tt.want {
				t.Errorf("levels = %q, want %q\n%+v", got, tt.want, st.Checks)
			}
			if st.Ready != tt.ready {
				t.Errorf("ready = %v, want %v", st.Ready, tt.ready)
			}
			for _, c := range st.Checks {
				if c.Level == LevelFail && c.Hint == "" && c.Command == "" && !strings.Contains(c.Detail, ":") {
					t.Errorf("failed check %q tells the user nothing to do: %+v", c.Title, c)
				}
			}
		})
	}
}

func TestMessagesTellWhatToDo(t *testing.T) {
	tests := []struct {
		name   string
		server fakeServer
		sys    check.System
		keySet bool
		title  string
		want   string
	}{
		{"brew command", fakeServer{models: videoModels()}, check.System{Chip: "M5", AppleSilicon: true, Launcher: "splash"}, true, "Splash installiert", "brew install incoai/tap/splash"},
		{"splash-m1 on M2", fakeServer{models: videoModels()}, check.System{Chip: "M2", AppleSilicon: true, Launcher: "splash-m1"}, true, "Splash installiert", "splash-m1"},
		{"address source", fakeServer{healthErr: errors.New("refused")}, m5Max(), true, "oMLX erreichbar", "~/.omlx/settings.json"},
		{"starting", fakeServer{healthErr: ErrStarting}, m5Max(), true, "oMLX erreichbar", "Kurz warten"},
		{"key required but none set", fakeServer{modelsErr: ErrUnauthorized}, m5Max(), false, "Zugriff auf oMLX", "keiner hinterlegt"},
		{"key rejected", fakeServer{modelsErr: ErrUnauthorized}, m5Max(), true, "Zugriff auf oMLX", "lehnt den API-Key ab"},
		{"oMLX version", fakeServer{models: []ServerModel{{ID: "x"}}}, m5Max(), true, "oMLX mit Splash-Unterstützung", OMLXReleases},
		{"oMLX version shown", fakeServer{models: videoModels()}, m5Max(), true, "oMLX erreichbar", "Version 0.7.1rc1"},
		{"oMLX version shown when down", fakeServer{healthErr: errors.New("refused")}, m5Max(), true, "oMLX erreichbar", "Version 0.7.1rc1"},
		{"Splash version shown", fakeServer{models: videoModels()}, m5Max(), true, "Splash installiert", "Version 1.5.0"},
		{"model reason", fakeServer{models: []ServerModel{{ID: "Qwen3.8-27B-oQ4e-mtp", Splash: splash(false, "Draft fehlt")}}}, m5Max(), true, "Splash-fähiges Modell", "Draft fehlt"},
		{"M3 required", fakeServer{models: videoModels()}, check.System{Chip: "M1", AppleSilicon: true, Launcher: "splash-m1", Splash: "1.4.0"}, true, "Chip", "M3 oder neuer"},
		{"community port is unofficial", fakeServer{models: videoModels()}, check.System{Chip: "M1", AppleSilicon: true, Launcher: "splash-m1", Splash: "1.4.0"}, true, "Chip", "inoffizielle"},
		{"community port link", fakeServer{models: videoModels()}, check.System{Chip: "M2", AppleSilicon: true, Launcher: "splash-m1"}, true, "Splash installiert", check.SplashM1Releases},
		{"macOS update", fakeServer{models: videoModels()}, check.System{Chip: "M5", AppleSilicon: true, MacOS: "26.3", Launcher: "splash", Splash: "1.5.0"}, true, "macOS", "Softwareupdate"},
		{"upgrade for Flash-Next", fakeServer{models: videoModels()}, check.System{Chip: "M5", AppleSilicon: true, MacOS: "27.0", Launcher: "splash", Splash: "1.3.2"}, true, "Splash installiert", "brew upgrade incoai/tap/splash"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := find(t, run(t, tt.server, tt.sys, tt.keySet), tt.title)
			text := c.Detail + " " + c.Hint + " " + c.Command
			if !strings.Contains(text, tt.want) {
				t.Errorf("%s: %q does not mention %q", tt.title, text, tt.want)
			}
		})
	}
}

func TestChoicesAndMemory(t *testing.T) {
	sys := m5Max()
	sys.MemoryGB = 32 // 24 GB usable: 27B (≈22 GB) fits, 35B-A3B (≈25 GB) does not
	models := append(videoModels(),
		ServerModel{ID: "Qwen3.8-27B-8bit", SizeBytes: 30 << 30, Splash: splash(true, "")},
		ServerModel{ID: "Qwen3.5-35B-A3B-4bit", Splash: splash(true, "")},
	)
	st := run(t, fakeServer{models: models}, sys, true)

	want := []struct {
		name        string
		recommended bool
		fits        bool
	}{
		{"Qwen3.8-27B", true, true},
		{"Qwen3.6-35B-A3B", true, false},
		{"Qwen3.8-27B-8bit", false, false},    // 30 GB + 3 GB overhead
		{"Qwen3.5-35B-A3B-4bit", false, true}, // size unknown: not judged
	}
	if len(st.Models) != len(want) {
		t.Fatalf("models = %+v", st.Models)
	}
	for i, w := range want {
		m := st.Models[i]
		if m.Name != w.name || m.Recommended != w.recommended || m.Fits != w.fits {
			t.Errorf("model %d = %+v, want %+v", i, m, w)
		}
	}
	mem := find(t, st, "Arbeitsspeicher")
	if mem.Level != LevelWarn || !strings.Contains(mem.Detail, "Qwen3.6-35B-A3B braucht ca. 25 GB") || !strings.Contains(mem.Detail, "ca. 24 GB für Modelle") {
		t.Errorf("memory check = %+v", mem)
	}
	if !st.Ready {
		t.Error("a warning must not block the start")
	}
}

func TestModelAdvice(t *testing.T) {
	unknownRAM := m5Max()
	unknownRAM.MemoryGB = 0
	small := m5Max()
	small.MemoryGB = 16

	tests := []struct {
		name         string
		models       []ServerModel
		sys          check.System
		wantHint     string
		wantCommands []string
	}{
		{"nothing loaded", nil, m5Max(), "Qwen3.8-27B", []string{
			"hf download Jundot/Qwen3.8-27B-oQ4e-mtp --local-dir ~/models/Qwen3.8-27B-oQ4e-mtp",
			"hf download incoai/Qwen3.8-27B-DFlash2 --local-dir ~/models/Qwen3.8-27B-DFlash2",
		}},
		{"target already listed", []ServerModel{{ID: "Qwen3.8-27B-oQ4e-mtp", Splash: splash(false, "Draft fehlt")}}, m5Max(), "oMLX neu starten", []string{
			"hf download incoai/Qwen3.8-27B-DFlash2 --local-dir ~/models/Qwen3.8-27B-DFlash2",
		}},
		{"RAM unknown", nil, unknownRAM, "Qwen3.8-27B", []string{
			"hf download Jundot/Qwen3.8-27B-oQ4e-mtp --local-dir ~/models/Qwen3.8-27B-oQ4e-mtp",
			"hf download incoai/Qwen3.8-27B-DFlash2 --local-dir ~/models/Qwen3.8-27B-DFlash2",
		}},
		{"too little RAM", nil, small, "reicht der Arbeitsspeicher nicht", nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			hint, command := modelAdvice(tt.models, tt.sys, "~/models")
			if !strings.Contains(hint, tt.wantHint) {
				t.Errorf("hint %q does not mention %q", hint, tt.wantHint)
			}
			if want := strings.Join(tt.wantCommands, "\n"); command != want {
				t.Errorf("command = %q, want %q", command, want)
			}
		})
	}
}

func TestNoModelOffersDownload(t *testing.T) {
	cfg := Config{BaseURL: "http://localhost:8000", ModelDir: "~/.omlx/models"}
	st := NewChecker(fakeServer{models: []ServerModel{{ID: "gemma-4", Splash: splash(false, "Qwen only")}}}, fixedSystem(m5Max()), cfg).Status(context.Background())
	c := find(t, st, "Splash-fähiges Modell")
	if c.Level != LevelFail || !strings.Contains(c.Command, "hf download Jundot/Qwen3.8-27B-oQ4e-mtp --local-dir ~/.omlx/models/") {
		t.Errorf("model check = %+v", c)
	}
}

func TestCatalog(t *testing.T) {
	sys := m5Max()
	sys.MemoryGB = 32 // 24 GB usable: 27B fits, 35B-A3B does not
	models := []ServerModel{{ID: "Qwen3.8-27B-oQ4e-mtp", Splash: splash(true, "")}}
	cfg := Config{BaseURL: "http://localhost:8000", ModelDir: "~/models"}
	st := NewChecker(fakeServer{models: models}, fixedSystem(sys), cfg).Status(context.Background())

	want := []struct {
		name    string
		fits    bool
		ready   bool
		inOMLX  bool
		command string
	}{
		{"Qwen3.8-27B", true, true, true, ""},
		{"Qwen3.6-35B-A3B", false, false, true, "hf download Jundot/Qwen3.6-35B-A3B-oQ4e-mtp --local-dir ~/models/Qwen3.6-35B-A3B-oQ4e-mtp\n" +
			"hf download incoai/Qwen3.6-35B-A3B-DFlash2 --local-dir ~/models/Qwen3.6-35B-A3B-DFlash2"},
		{"Qwen3.8-Flash-Next", false, false, true, "hf download ISTA-DASLab/Qwen3.8-Flash-Next-GSQ-RCO-GGUF " +
			"Q2_0/Qwen3.8-Flash-Next-GSQ-RCO-Q2_0-00001-of-00002.gguf Q2_0/Qwen3.8-Flash-Next-GSQ-RCO-Q2_0-00002-of-00002.gguf " +
			"mmproj-Qwen3.8-Flash-Next-BF16.gguf --local-dir ~/models/Qwen3.8-Flash-Next-GSQ-RCO-GGUF\n" +
			"hf download incoai/Qwen3.8-Flash-Next-DFlash2 --local-dir ~/models/Qwen3.8-Flash-Next-DFlash2"},
		{"Ornith-1.5-122B", false, false, true, "hf download ornith-ai/Ornith-1.5-122B-MLX-4bit --local-dir ~/models/Ornith-1.5-122B-MLX-4bit\n" +
			"hf download incoai/Ornith-1.5-122B-DFlash2 --local-dir ~/models/Ornith-1.5-122B-DFlash2"},
		{"Flint", false, false, true, "hf download unsloth/flint-GGUF flint-UD-Q4_K_XL.gguf --local-dir ~/models/flint-GGUF\n" +
			"hf download incoai/Flint-DFlash2 --local-dir ~/models/Flint-DFlash2"},
	}
	if len(st.Catalog) != len(want) {
		t.Fatalf("catalog = %+v", st.Catalog)
	}
	for i, w := range want {
		e := st.Catalog[i]
		if e.Name != w.name || e.Fits != w.fits || e.Ready != w.ready || e.InOMLX != w.inOMLX || e.Command != w.command {
			t.Errorf("entry %d = %+v, want %+v", i, e, w)
		}
	}
	if !strings.Contains(st.CatalogAsOf, check.CatalogAsOf) || !strings.Contains(st.CatalogAsOf, "oMLX "+check.CatalogOMLX) {
		t.Errorf("catalog as of = %q", st.CatalogAsOf)
	}
}

func TestCatalogNamesRequiredSplash(t *testing.T) {
	sys := m5Max()
	sys.Splash = "1.3.2"
	st := run(t, fakeServer{models: videoModels()}, sys, true)
	for _, e := range st.Catalog {
		needs := strings.Contains(e.Advice, "(installiert: 1.3.2)")
		if needs != (e.Name != "Qwen3.8-27B" && e.Name != "Qwen3.6-35B-A3B") {
			t.Errorf("%s: advice %q", e.Name, e.Advice)
		}
	}
}

func TestCatalogWithoutOMLX(t *testing.T) {
	cfg := Config{BaseURL: "http://localhost:8000", ModelDir: "~/models"}
	st := NewChecker(fakeServer{healthErr: errors.New("refused")}, fixedSystem(m5Max()), cfg).Status(context.Background())
	if len(st.Catalog) != len(check.Catalog) {
		t.Fatalf("catalog = %+v", st.Catalog)
	}
	for _, e := range st.Catalog {
		if e.Ready || (e.Command == "") == e.InOMLX {
			t.Errorf("entry %+v: without oMLX every oMLX model needs a download, the others none", e)
		}
	}
}

// oMLX names the missing draft, so a family the catalog does not know yet
// still shows up with a download command.
func TestDraftFromOMLX(t *testing.T) {
	models := append(videoModels(),
		ServerModel{ID: "Qwen3.8-Flash-Next-GSQ-RCO-Q2_0", Splash: missingDraft("incoai/Qwen3.8-Flash-Next-DFlash2")},
		ServerModel{ID: "Future-9B-4bit", SizeBytes: 5 << 30, Splash: missingDraft("incoai/Future-9B-DFlash2")},
		ServerModel{ID: "llama-4", Splash: splash(false, "Splash serves Qwen only")},
	)
	cfg := Config{BaseURL: "http://localhost:8000", ModelDir: "~/models"}
	st := NewChecker(fakeServer{models: models}, fixedSystem(m5Max()), cfg).Status(context.Background())

	byID := map[string]ModelChoice{}
	for _, m := range st.Models {
		byID[m.ID] = m
	}
	if _, ok := byID["llama-4"]; ok {
		t.Error("a model oMLX cannot hand to Splash must not be offered")
	}
	future, ok := byID["Future-9B-4bit"]
	if !ok || future.Ready || future.Command != "hf download incoai/Future-9B-DFlash2 --local-dir ~/models/Future-9B-DFlash2" {
		t.Errorf("unknown family = %+v", future)
	}
	if next := byID["Qwen3.8-Flash-Next-GSQ-RCO-Q2_0"]; next.Name != "Qwen3.8-Flash-Next" || next.Ready || next.Command == "" {
		t.Errorf("Flash-Next = %+v", next)
	}

	model := find(t, st, "Splash-fähiges Modell")
	if model.Level != LevelWarn || !strings.Contains(model.Command, "incoai/Future-9B-DFlash2") || !strings.Contains(model.Command, "incoai/Qwen3.8-Flash-Next-DFlash2") {
		t.Errorf("model check = %+v", model)
	}
	for _, e := range st.Catalog {
		if e.Name == "Qwen3.8-Flash-Next" && e.Command != "hf download incoai/Qwen3.8-Flash-Next-DFlash2 --local-dir ~/models/Qwen3.8-Flash-Next-DFlash2" {
			t.Errorf("Flash-Next catalog command = %q, want the draft only", e.Command)
		}
	}
}

func TestOnlyDraftsMissing(t *testing.T) {
	models := []ServerModel{{ID: "Qwen3.8-27B-oQ4e-mtp", Splash: missingDraft("incoai/Qwen3.8-27B-DFlash2")}}
	cfg := Config{BaseURL: "http://localhost:8000", ModelDir: "~/models"}
	st := NewChecker(fakeServer{models: models}, fixedSystem(m5Max()), cfg).Status(context.Background())
	c := find(t, st, "Splash-fähiges Modell")
	if c.Level != LevelFail || c.Command != "hf download incoai/Qwen3.8-27B-DFlash2 --local-dir ~/models/Qwen3.8-27B-DFlash2" {
		t.Errorf("model check = %+v", c)
	}
}

func TestDiagnosis(t *testing.T) {
	cfg := Config{BaseURL: "http://localhost:8000", KeySet: true, AppVersion: "1.2.0"}
	ready := NewChecker(fakeServer{models: videoModels()}, fixedSystem(m5Max()), cfg).Status(context.Background())
	down := NewChecker(fakeServer{healthErr: errors.New("refused")}, fixedSystem(check.System{}), Config{}).Status(context.Background())
	tests := []struct {
		name string
		st   Status
		want []string
	}{
		{"versions and system", ready, []string{"Splash Bench: 1.2.0", "Chip: M5 Max", "Arbeitsspeicher: 128 GB", "macOS: 27.0.1", "Splash (splash): 1.5.0", "oMLX: 0.7.1rc1"}},
		{"every check with its mark", ready, []string{"✓ Splash installiert: Version 1.5.0", "✓ Arbeitsspeicher:"}},
		{"unknown values named", down, []string{"Splash Bench: unbekannt", "Chip: unbekannt", "Arbeitsspeicher: unbekannt", "oMLX: unbekannt", "✗ oMLX erreichbar:", "– Zugriff auf oMLX:"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for _, w := range tt.want {
				if !strings.Contains(tt.st.Diagnosis, w) {
					t.Errorf("diagnosis lacks %q:\n%s", w, tt.st.Diagnosis)
				}
			}
		})
	}
}
