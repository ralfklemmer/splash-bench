package check

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os/exec"
	"strings"
	"testing"
	"testing/fstest"
)

const home = "/Users/ralf"

type fakeCommands map[string]string // "name args" -> output; missing = not installed

func (f fakeCommands) Output(_ context.Context, name string, args ...string) (string, error) {
	key := strings.TrimSpace(name + " " + strings.Join(args, " "))
	out, ok := f[key]
	if !ok {
		return "", fmt.Errorf("%s: %w", name, exec.ErrNotFound)
	}
	return out, nil
}

func healthyMac() fakeCommands {
	return fakeCommands{
		"sysctl -n machdep.cpu.brand_string": "Apple M5 Max\n",
		"sysctl -n hw.memsize":               "137438953472\n",
		"sw_vers -productVersion":            "27.0.1\n",
		"splash --version":                   "Splash 1.5.0\n",
		"hf version":                         "1.0.0",
	}
}

type fakeServer struct {
	healthErr error
	modelsErr error
	models    []ServerModel
	listed    bool
}

func (s *fakeServer) Health(context.Context) error { return s.healthErr }

func (s *fakeServer) Models(context.Context) ([]ServerModel, error) {
	s.listed = true
	return s.models, s.modelsErr
}

const modelRoot = "Users/ralf/.omlx/models/mlx-community"

func settingsFS() fstest.MapFS {
	return fstest.MapFS{
		"Users/ralf/.omlx/settings.json": {Data: []byte(`{"auth":{"api_key":"secret"},"model":{"model_dirs":["/Users/ralf/.omlx/models/mlx-community"]}}`)},
		modelRoot:                        {Mode: fs.ModeDir | 0o755},
	}
}

// completeFS holds target and draft of the two models in the video.
func completeFS() fstest.MapFS {
	fsys := settingsFS()
	for _, m := range OMLXCatalog()[:2] {
		fsys[modelRoot+"/"+string(m.Target)+"/config.json"] = &fstest.MapFile{Data: []byte("{}")}
		fsys[modelRoot+"/"+string(m.Draft)+"/config.json"] = &fstest.MapFile{Data: []byte("{}")}
	}
	return fsys
}

func compatibleModels() []ServerModel {
	return []ServerModel{
		{ID: "Qwen3.8-27B-oQ4e-mtp", Engine: "splash", Splash: &SplashSupport{Compatible: true}},
		{ID: "Qwen3.6-35B-A3B-oQ4e-mtp", Splash: &SplashSupport{Compatible: true}},
	}
}

func runCheck(t *testing.T, commands fakeCommands, fsys fstest.MapFS, server *fakeServer, keySet bool) (Report, string) {
	t.Helper()
	report := NewChecker(commands, fsys, server, Config{BaseURL: "http://localhost:8000", Home: home, APIKeySet: keySet}).Run(context.Background())
	var out bytes.Buffer
	if err := (Renderer{}).Render(&out, report); err != nil {
		t.Fatal(err)
	}
	return report, out.String()
}

func TestCheckerHealthySetupHasNoProblems(t *testing.T) {
	report, out := runCheck(t, healthyMac(), completeFS(), &fakeServer{models: compatibleModels()}, true)
	if report.Count(StatusFail) != 0 || report.Count(StatusWarn) != 0 {
		t.Errorf("want clean report, got:\n%s", out)
	}
	for _, want := range []string{"✓ Chip: Apple M5 Max", "✓ splash 1.5.0", "Splash kompatibel, aktive Engine: Splash", "bis 256K Kontext (≈ 29 GB"} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in:\n%s", want, out)
		}
	}
	for _, want := range []string{"· nicht installiert (optional", "· Flint: bis 256K Kontext", "→ hf download unsloth/flint-GGUF flint-UD-Q4_K_XL.gguf", "· Katalog-Stand 29.10.2026"} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in:\n%s", want, out)
		}
	}
	if len(report.Commands) != 0 {
		t.Errorf("commands = %v, want none: unmeasured models are only hints", report.Commands)
	}
}

func TestCheckerWithoutKeySkipsServerModels(t *testing.T) {
	server := &fakeServer{models: compatibleModels()}
	_, out := runCheck(t, healthyMac(), completeFS(), server, false)
	if server.listed {
		t.Error("model list requested without API key")
	}
	if !strings.Contains(out, "! Kein OMLX_API_KEY gesetzt") {
		t.Errorf("missing key warning in:\n%s", out)
	}
}

func TestCheckerUnreachableServer(t *testing.T) {
	server := &fakeServer{healthErr: errors.New("connection refused")}
	report, out := runCheck(t, healthyMac(), completeFS(), server, true)
	if server.listed || report.Count(StatusFail) != 1 || !strings.Contains(out, "✗ Server nicht erreichbar") {
		t.Errorf("want one failure for the server, got:\n%s", out)
	}
	if !strings.Contains(out, "✓ Zielmodell Jundot/Qwen3.8-27B-oQ4e-mtp") {
		t.Errorf("local checks must still run:\n%s", out)
	}
}

func TestCheckerReportsServerErrorAndIncompatibility(t *testing.T) {
	_, out := runCheck(t, healthyMac(), completeFS(), &fakeServer{modelsErr: errors.New("HTTP 401")}, true)
	if !strings.Contains(out, "✗ Modellliste nicht abrufbar: HTTP 401") {
		t.Errorf("missing list error in:\n%s", out)
	}

	models := []ServerModel{{ID: "Qwen3.8-27B-oQ4e-mtp", Splash: &SplashSupport{Reason: "Splash needs the Qwen3.8-27B DFlash2 draft in a model directory"}}}
	_, out = runCheck(t, healthyMac(), completeFS(), &fakeServer{models: models}, true)
	for _, want := range []string{"✗ oMLX: Splash nicht möglich – Splash needs the Qwen3.8-27B DFlash2 draft", "! oMLX: Qwen3.6-35B-A3B-oQ4e-mtp nicht in der Modellliste"} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in:\n%s", want, out)
		}
	}
}

func TestCheckerSplashLauncher(t *testing.T) {
	tests := []struct {
		name    string
		chip    string
		splash  string // "" = not installed
		m1      string
		want    string
		wantErr int
	}{
		{"missing", "Apple M5 Max", "", "", "✗ splash nicht gefunden", 1},
		{"too old", "Apple M4", "Splash 1.2.9", "", "✗ splash 1.2.9 installiert – zu alt", 1},
		{"no thinking budget", "Apple M4", "Splash 1.3.1", "", "! splash 1.3.1 installiert – es fehlt: Thinking-Budget", 0},
		{"no Flash-Next", "Apple M4", "Splash 1.3.2", "", "! splash 1.3.2 installiert – es fehlt: Qwen3.8-Flash-Next (ab 1.4.0)", 0},
		{"m2 needs splash-m1", "Apple M2 Pro", "Splash 1.3.2", "", "✗ splash-m1 nicht gefunden – auf M1/M2 nötig (splash allein reicht hier nicht)", 1},
		{"m2 points to community port", "Apple M2 Pro", "", "", SplashM1Releases, 1},
		{"m1 with splash-m1", "Apple M1", "", "Splash 1.4.0", "✓ splash-m1 1.4.0 installiert", 0},
		{"m1 warns about chip", "Apple M1", "", "Splash 1.4.0", "! Chip: Apple M1 – Splash von Inco AI braucht M3 oder neuer", 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			commands := healthyMac()
			commands["sysctl -n machdep.cpu.brand_string"] = tt.chip
			delete(commands, "splash --version")
			if tt.splash != "" {
				commands["splash --version"] = tt.splash
			}
			if tt.m1 != "" {
				commands["splash-m1 --version"] = tt.m1
			}
			report, out := runCheck(t, commands, completeFS(), &fakeServer{models: compatibleModels()}, true)
			if !strings.Contains(out, tt.want) {
				t.Errorf("missing %q in:\n%s", tt.want, out)
			}
			if got := report.Count(StatusFail); got != tt.wantErr {
				t.Errorf("failures = %d, want %d", got, tt.wantErr)
			}
		})
	}
}

func TestCheckerIntelMac(t *testing.T) {
	commands := healthyMac()
	commands["sysctl -n machdep.cpu.brand_string"] = "Intel(R) Core(TM) i7"
	_, out := runCheck(t, commands, completeFS(), &fakeServer{}, false)
	if !strings.Contains(out, "✗ Chip: Intel") {
		t.Errorf("want Intel failure in:\n%s", out)
	}
}

func TestCheckerMissingModelsYieldDownloadCommands(t *testing.T) {
	fsys := settingsFS()
	fsys[modelRoot+"/Jundot/Qwen3.8-27B-oQ4e-mtp/config.json"] = &fstest.MapFile{Data: []byte("{}")}
	fsys[modelRoot+"/incoai/Qwen3.8-27B-DFlash2/model.safetensors"] = &fstest.MapFile{Data: []byte("x")} // no config.json
	commands := healthyMac()
	delete(commands, "hf version")

	report, out := runCheck(t, commands, fsys, &fakeServer{}, false)
	want := []string{
		"brew install huggingface-cli   # liefert den Befehl hf",
		"hf download incoai/Qwen3.8-27B-DFlash2 --local-dir ~/.omlx/models/mlx-community/incoai/Qwen3.8-27B-DFlash2",
		"hf download Jundot/Qwen3.6-35B-A3B-oQ4e-mtp --local-dir ~/.omlx/models/mlx-community/Jundot/Qwen3.6-35B-A3B-oQ4e-mtp",
		"hf download incoai/Qwen3.6-35B-A3B-DFlash2 --local-dir ~/.omlx/models/mlx-community/incoai/Qwen3.6-35B-A3B-DFlash2",
		"# danach oMLX neu starten (oder POST /admin/api/reload – entlädt alle Modelle)",
	}
	if strings.Join(report.Commands, "\n") != strings.Join(want, "\n") {
		t.Errorf("commands =\n%s\nwant\n%s", strings.Join(report.Commands, "\n"), strings.Join(want, "\n"))
	}
	if !strings.Contains(out, "✗ Draft (DFlash2) incoai/Qwen3.8-27B-DFlash2 unvollständig") {
		t.Errorf("missing incomplete finding in:\n%s", out)
	}
}

func TestCheckerOnlySuggestsModelsThatFit(t *testing.T) {
	commands := healthyMac()
	commands["sysctl -n hw.memsize"] = fmt.Sprint(32 << 30)
	report, out := runCheck(t, commands, settingsFS(), &fakeServer{}, false)
	for _, c := range report.Commands {
		if strings.Contains(c, "35B") {
			t.Errorf("35B does not fit into 32 GB, but got %q", c)
		}
	}
	if !strings.Contains(out, "· Qwen3.6-35B-A3B: passt nicht") {
		t.Errorf("want 35B not fitting in:\n%s", out)
	}
}

func TestCheckerWarnsAboutFullSplashPackage(t *testing.T) {
	fsys := completeFS()
	fsys[modelRoot+"/incoai/Qwen3.8-27B-Splash/manifest.json"] = &fstest.MapFile{Data: make([]byte, 2000)}
	report, out := runCheck(t, healthyMac(), fsys, &fakeServer{}, false)
	if !strings.Contains(out, "! Splash-Komplettpaket incoai/Qwen3.8-27B-Splash gefunden") || !strings.Contains(out, "Keine config.json") {
		t.Errorf("missing full package warning in:\n%s", out)
	}
	if report.Count(StatusFail) != 0 {
		t.Errorf("full package must only warn:\n%s", out)
	}
}

func TestCheckerFallsBackWithoutSettings(t *testing.T) {
	fsys := fstest.MapFS{}
	_, out := runCheck(t, healthyMac(), fsys, &fakeServer{}, false)
	if !strings.Contains(out, "! oMLX-Einstellungen nicht lesbar, nehme ~/.omlx/models") {
		t.Errorf("missing settings fallback in:\n%s", out)
	}
}

func TestCheckerOldMacOS(t *testing.T) {
	commands := healthyMac()
	commands["sw_vers -productVersion"] = "26.3\n"
	report, out := runCheck(t, commands, completeFS(), &fakeServer{models: compatibleModels()}, true)
	if !strings.Contains(out, "✗ macOS: 26.3 – Splash braucht macOS 26.4 oder neuer") || report.Count(StatusFail) != 1 {
		t.Errorf("want macOS failure in:\n%s", out)
	}
}

func TestCheckerNoModelFits(t *testing.T) {
	commands := healthyMac()
	commands["sysctl -n hw.memsize"] = fmt.Sprint(16 << 30)
	report, out := runCheck(t, commands, settingsFS(), &fakeServer{}, false)
	if !strings.Contains(out, "✗ Kein Splash-Modell für oMLX passt") || len(report.Commands) != 0 {
		t.Errorf("want no-fit failure without commands in:\n%s", out)
	}
}

func TestCheckerFindsGGUFTarget(t *testing.T) {
	fsys := completeFS()
	flint := modelRoot + "/unsloth/flint-GGUF"
	fsys[flint+"/flint-UD-Q4_K_XL.gguf"] = &fstest.MapFile{Data: []byte("GGUF")}
	fsys[modelRoot+"/incoai/Flint-DFlash2/config.json"] = &fstest.MapFile{Data: []byte("{}")}
	models := append(compatibleModels(), ServerModel{ID: "flint-UD-Q4_K_XL", Splash: &SplashSupport{Compatible: true}})
	report, out := runCheck(t, healthyMac(), fsys, &fakeServer{models: models}, true)
	for _, want := range []string{"✓ Zielmodell unsloth/flint-GGUF", "✓ Draft (DFlash2) incoai/Flint-DFlash2"} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in:\n%s", want, out)
		}
	}
	if report.Count(StatusFail) != 0 || report.Count(StatusWarn) != 0 {
		t.Errorf("want clean report, got:\n%s", out)
	}
}

func TestCheckerIncompleteGGUF(t *testing.T) {
	fsys := completeFS()
	fsys[modelRoot+"/unsloth/flint-GGUF/flint-UD-Q4_K_XL.gguf.incomplete"] = &fstest.MapFile{Data: []byte("x")}
	_, out := runCheck(t, healthyMac(), fsys, &fakeServer{}, false)
	if !strings.Contains(out, "✗ Zielmodell unsloth/flint-GGUF unvollständig") {
		t.Errorf("want incomplete GGUF in:\n%s", out)
	}
}
