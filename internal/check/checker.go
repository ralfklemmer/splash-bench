// Package check diagnoses whether a Mac is ready for Splash in oMLX:
// hardware, Splash launcher, oMLX server, model folders and memory.
package check

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os/exec"
	"path"
	"strings"
)

// CommandRunner runs a local program and returns its standard output.
// A missing program must yield an error wrapping exec.ErrNotFound.
type CommandRunner interface {
	Output(ctx context.Context, name string, args ...string) (string, error)
}

// Server is the part of oMLX the diagnosis needs.
type Server interface {
	Health(ctx context.Context) error
	Models(ctx context.Context) ([]ServerModel, error)
}

// ServerModel is a model as oMLX reports it.
type ServerModel struct {
	ID     string
	Engine string // "" means oMLX's own engine
	// Splash is nil when oMLX reports no Splash information for the model.
	Splash *SplashSupport
}

// SplashSupport is oMLX's verdict whether Splash can serve a model.
type SplashSupport struct {
	Compatible bool
	Reason     string
}

// Config holds what the user passed on the command line.
type Config struct {
	BaseURL   string
	Home      string
	APIKeySet bool
}

// Checker runs the diagnosis. It only reads; it never changes settings.
type Checker struct {
	commands CommandRunner
	files    fs.FS
	server   Server
	cfg      Config
}

func NewChecker(commands CommandRunner, files fs.FS, server Server, cfg Config) *Checker {
	return &Checker{commands: commands, files: files, server: server, cfg: cfg}
}

// facts collects what the checks learn for later sections.
type facts struct {
	chip      Chip
	memory    Memory
	memoryOK  bool
	modelDirs []string
	reachable bool
	models    map[string]ServerModel
	modelsOK  bool
}

// Run performs all checks and returns the report.
func (c *Checker) Run(ctx context.Context) Report {
	var f facts
	var r Report
	r.Sections = append(r.Sections,
		c.systemSection(ctx, &f),
		c.splashSection(ctx, f.chip),
		c.serverSection(ctx, &f),
	)
	r.Sections = append(r.Sections, c.modelSections(f)...)
	recommendations, commands := c.recommendationSection(ctx, f)
	r.Sections = append(r.Sections, recommendations)
	r.Commands = commands
	return r
}

func (c *Checker) output(ctx context.Context, name string, args ...string) (string, error) {
	out, err := c.commands.Output(ctx, name, args...)
	return strings.TrimSpace(out), err
}

func (c *Checker) systemSection(ctx context.Context, f *facts) Section {
	s := Section{Title: "System"}
	s.Findings = append(s.Findings, c.chipFinding(ctx, f), c.memoryFinding(ctx, f), c.macOSFinding(ctx, f.chip))
	return s
}

func (c *Checker) chipFinding(ctx context.Context, f *facts) Finding {
	brand, err := c.output(ctx, "sysctl", "-n", "machdep.cpu.brand_string")
	if err != nil {
		return Finding{Status: StatusWarn, Text: "Chip nicht lesbar: " + err.Error()}
	}
	f.chip = Chip{Brand: brand}
	if !f.chip.AppleSilicon() {
		return Finding{Status: StatusFail, Text: "Chip: " + brand,
			Hints: []string{"MLX und Splash laufen nur auf Apple Silicon; Splash von Inco AI braucht M3 oder neuer."}}
	}
	if !f.chip.OfficialSplash() {
		return Finding{Status: StatusWarn, Text: "Chip: " + brand + " – Splash von Inco AI braucht M3 oder neuer",
			Hints: []string{"Auf M1/M2 geht nur der inoffizielle Community-Port splash-m1 (ohne Support von Inco AI): " + SplashM1Releases}}
	}
	return Finding{Status: StatusOK, Text: "Chip: " + brand}
}

func (c *Checker) memoryFinding(ctx context.Context, f *facts) Finding {
	out, err := c.output(ctx, "sysctl", "-n", "hw.memsize")
	if err != nil {
		return Finding{Status: StatusWarn, Text: "Arbeitsspeicher nicht lesbar: " + err.Error()}
	}
	mem, err := ParseMemory(out)
	if err != nil {
		return Finding{Status: StatusWarn, Text: fmt.Sprintf("Arbeitsspeicher nicht lesbar: %q", out)}
	}
	f.memory, f.memoryOK = mem, true
	return Finding{Status: StatusOK, Text: fmt.Sprintf("Arbeitsspeicher: %.0f GB (davon ca. %.0f GB für Modelle nutzbar)", mem.GB(), mem.UsableGB())}
}

func (c *Checker) macOSFinding(ctx context.Context, chip Chip) Finding {
	out, err := c.output(ctx, "sw_vers", "-productVersion")
	if err != nil {
		return Finding{Status: StatusWarn, Text: "macOS-Version nicht lesbar: " + err.Error()}
	}
	return macOSRule(out, chip)
}

// macOSRule grades the macOS version: the official Splash installs from
// MinMacOS on; for the community port splash-m1 that is only a warning.
func macOSRule(version string, chip Chip) Finding {
	v, err := ParseMacOSVersion(version)
	if err != nil {
		return Finding{Status: StatusWarn, Text: "macOS: unbekannte Version " + version}
	}
	if !v.Less(MinMacOS) {
		return Finding{Status: StatusOK, Text: "macOS: " + version}
	}
	text := fmt.Sprintf("macOS: %s – Splash braucht macOS %d.%d oder neuer", version, MinMacOS.Major, MinMacOS.Minor)
	if chip.NeedsSplashM1() {
		return Finding{Status: StatusWarn, Text: text, Hints: []string{"Ob splash-m1 auf älterem macOS läuft, ist nicht geprüft."}}
	}
	return Finding{Status: StatusFail, Text: text, Hints: []string{"macOS aktualisieren (Systemeinstellungen → Allgemein → Softwareupdate)."}}
}

func (c *Checker) splashSection(ctx context.Context, chip Chip) Section {
	s := Section{Title: "Splash"}
	launchers := []string{"splash", "splash-m1"}
	found := map[string]bool{}
	for _, name := range launchers {
		finding, ok := c.launcherFinding(ctx, name)
		if ok {
			found[name] = true
			s.Findings = append(s.Findings, finding)
		}
	}
	if needed := chip.Launcher(); !found[needed] && (chip.AppleSilicon() || len(found) == 0) {
		s.Findings = append(s.Findings, missingLauncherFinding(needed, len(found) > 0))
	}
	return s
}

// launcherFinding runs `<name> --version`; ok is false when the program is
// not installed at all.
func (c *Checker) launcherFinding(ctx context.Context, name string) (Finding, bool) {
	out, err := c.output(ctx, name, "--version")
	if errors.Is(err, exec.ErrNotFound) {
		return Finding{}, false
	}
	if err != nil {
		return Finding{Status: StatusFail, Text: name + " --version schlägt fehl: " + err.Error()}, true
	}
	version, err := ParseVersion(out)
	if err != nil {
		return Finding{Status: StatusWarn, Text: name + ": unbekannte Version " + out}, true
	}
	return versionFinding(name, version), true
}

func versionFinding(name string, v Version) Finding {
	text := fmt.Sprintf("%s %s installiert", name, v)
	if v.Less(MinSplashVersion) {
		return Finding{Status: StatusFail, Text: text + fmt.Sprintf(" – zu alt, oMLX braucht mindestens %s", MinSplashVersion),
			Hints: []string{upgradeHint(name)}}
	}
	missing := v.MissingFeatures()
	if len(missing) > 0 {
		return Finding{Status: StatusWarn, Text: text + " – es fehlt: " + strings.Join(missing, ", "),
			Hints: []string{upgradeHint(name)}}
	}
	return Finding{Status: StatusOK, Text: text + " – alle Funktionen (Mixed-Bit, Thinking-Budget, Qwen3.8-Flash-Next)"}
}

// upgradeHint says how to update the launcher: Homebrew for Inco AI's
// Splash, the community releases for splash-m1.
func upgradeHint(name string) string {
	if name == "splash-m1" {
		return "Neue Version von " + SplashM1Releases + " installieren"
	}
	return "brew update && brew upgrade incoai/tap/splash"
}

func missingLauncherFinding(needed string, otherFound bool) Finding {
	if needed == "splash-m1" {
		text := "splash-m1 nicht gefunden – auf M1/M2 nötig"
		if otherFound {
			text += " (splash allein reicht hier nicht)"
		}
		return Finding{Status: StatusFail, Text: text, Hints: []string{
			"Inoffiziellen Community-Port splash-m1 installieren (nicht von Inco AI): " + SplashM1Releases,
			"danach: splash-m1 --version prüfen"}}
	}
	return Finding{Status: StatusFail, Text: "splash nicht gefunden",
		Hints: []string{"brew install incoai/tap/splash", "danach: splash --version (mindestens " + MinSplashVersion.String() + ")"}}
}

func (c *Checker) serverSection(ctx context.Context, f *facts) Section {
	s := Section{Title: "oMLX"}
	s.Findings = append(s.Findings, c.healthFinding(ctx, f))
	s.Findings = append(s.Findings, c.modelDirFindings(f)...)
	if f.reachable {
		s.Findings = append(s.Findings, c.modelListFinding(ctx, f))
	}
	return s
}

func (c *Checker) healthFinding(ctx context.Context, f *facts) Finding {
	if err := c.server.Health(ctx); err != nil {
		return Finding{Status: StatusFail, Text: "Server nicht erreichbar: " + c.cfg.BaseURL,
			Hints: []string{"oMLX-App starten (Server auf Port 8000) oder -base-url angeben.", err.Error()}}
	}
	f.reachable = true
	return Finding{Status: StatusOK, Text: "Server erreichbar: " + c.cfg.BaseURL}
}

func (c *Checker) modelListFinding(ctx context.Context, f *facts) Finding {
	if !c.cfg.APIKeySet {
		return Finding{Status: StatusWarn, Text: "Kein OMLX_API_KEY gesetzt – Splash-Kompatibilität laut oMLX nicht geprüft",
			Hints: []string{"Key steht in ~/.omlx/settings.json (auth.api_key); nur an den lokalen oMLX-Server gesendet."}}
	}
	models, err := c.server.Models(ctx)
	if err != nil {
		return Finding{Status: StatusFail, Text: "Modellliste nicht abrufbar: " + err.Error(),
			Hints: []string{"API-Key prüfen (HTTP 401 = falscher Key)."}}
	}
	f.models, f.modelsOK = map[string]ServerModel{}, true
	for _, m := range models {
		f.models[m.ID] = m
	}
	return Finding{Status: StatusOK, Text: fmt.Sprintf("oMLX kennt %d Modelle", len(models))}
}

func (c *Checker) modelDirFindings(f *facts) []Finding {
	dirs, err := ReadModelDirs(c.files, c.cfg.Home)
	if err != nil {
		dirs = []string{DefaultModelDir(c.cfg.Home)}
		f.modelDirs = dirs
		return []Finding{{Status: StatusWarn, Text: "oMLX-Einstellungen nicht lesbar, nehme " + ShellPath(dirs[0], c.cfg.Home), Hints: []string{err.Error()}}}
	}
	if len(dirs) == 0 {
		dirs = []string{DefaultModelDir(c.cfg.Home)}
	}
	f.modelDirs = dirs
	findings := make([]Finding, 0, len(dirs))
	for _, d := range dirs {
		if DirExists(c.files, d) {
			findings = append(findings, Finding{Status: StatusOK, Text: "Modellordner: " + ShellPath(d, c.cfg.Home)})
		} else {
			findings = append(findings, Finding{Status: StatusFail, Text: "Modellordner fehlt: " + ShellPath(d, c.cfg.Home)})
		}
	}
	return findings
}

func (c *Checker) modelSections(f facts) []Section {
	catalog := OMLXCatalog()
	sections := make([]Section, 0, len(catalog))
	for _, m := range catalog {
		s := Section{Title: "Modell " + m.Family}
		if !c.anyFolder(m, f.modelDirs) {
			s.Findings = append(s.Findings, Finding{Status: StatusInfo,
				Text: fmt.Sprintf("nicht installiert (optional, Zielmodell + Draft ≈ %s)", formatGB(m.TargetGB+m.DraftGB))})
			sections = append(sections, s)
			continue
		}
		s.Findings = append(s.Findings,
			c.folderFinding("Zielmodell", m.Target, m.TargetGB, LocateTarget(c.files, f.modelDirs, m)),
			c.folderFinding("Draft (DFlash2)", m.Draft, m.DraftGB, Locate(c.files, f.modelDirs, m.Draft)),
		)
		if pkg, ok := c.fullPackageFinding(m.FullPackage, f.modelDirs); m.FullPackage != "" && ok {
			s.Findings = append(s.Findings, pkg)
		}
		if f.modelsOK {
			s.Findings = append(s.Findings, serverModelFinding(m.TargetID(), f.models))
		}
		sections = append(sections, s)
	}
	return sections
}

// anyFolder reports whether target, draft or full package of the model lies
// in a model directory, complete or not.
func (c *Checker) anyFolder(m TargetModel, dirs []string) bool {
	if LocateTarget(c.files, dirs, m).State != FolderMissing || Locate(c.files, dirs, m.Draft).State != FolderMissing {
		return true
	}
	if m.FullPackage == "" {
		return false
	}
	for _, d := range dirs {
		if DirExists(c.files, path.Join(d, string(m.FullPackage))) {
			return true
		}
	}
	return false
}

func (c *Checker) folderFinding(role string, repo Repo, sizeGB float64, loc Location) Finding {
	switch loc.State {
	case FolderComplete:
		return Finding{Status: StatusOK, Text: fmt.Sprintf("%s %s (%s)", role, repo, ShellPath(loc.Path, c.cfg.Home))}
	case FolderIncomplete:
		return Finding{Status: StatusFail, Text: fmt.Sprintf("%s %s unvollständig in %s", role, repo, ShellPath(loc.Path, c.cfg.Home)),
			Hints: []string{"Download wiederholen (Befehl unten)."}}
	default:
		return Finding{Status: StatusWarn, Text: fmt.Sprintf("%s %s fehlt (%s)", role, repo, formatGB(sizeGB))}
	}
}

func (c *Checker) fullPackageFinding(pkg Repo, dirs []string) (Finding, bool) {
	for _, d := range dirs {
		dir := path.Join(d, string(pkg))
		if !DirExists(c.files, dir) {
			continue
		}
		size, _ := DirSizeGB(c.files, dir)
		return Finding{Status: StatusWarn,
			Text: fmt.Sprintf("Splash-Komplettpaket %s gefunden (%s) – für oMLX unbrauchbar", pkg, formatGB(size)),
			Hints: []string{
				"Keine config.json: oMLX lädt es nicht, das Modell bleibt unsichtbar.",
				"oMLX braucht Zielmodell + DFlash2-Draft. Das Paket nur für eigenständiges splash serve behalten.",
				"Ordner: " + ShellPath(dir, c.cfg.Home),
			}}, true
	}
	return Finding{}, false
}

func serverModelFinding(id string, models map[string]ServerModel) Finding {
	m, ok := models[id]
	if !ok {
		return Finding{Status: StatusWarn, Text: "oMLX: " + id + " nicht in der Modellliste",
			Hints: []string{"Nach dem Download oMLX neu einlesen: App neu starten oder POST /admin/api/reload (entlädt alle Modelle)."}}
	}
	engine := engineLabel(m.Engine)
	switch {
	case m.Splash == nil:
		return Finding{Status: StatusWarn, Text: "oMLX: keine Splash-Angaben für " + id + " (oMLX-Version zu alt?)"}
	case m.Splash.Compatible:
		return Finding{Status: StatusOK, Text: fmt.Sprintf("oMLX: Splash kompatibel, aktive Engine: %s", engine)}
	default:
		return Finding{Status: StatusFail, Text: "oMLX: Splash nicht möglich – " + m.Splash.Reason,
			Hints: []string{"Draft fehlt oder oMLX hat ihn noch nicht eingelesen (Neustart oder POST /admin/api/reload)."}}
	}
}

func engineLabel(engine string) string {
	switch engine {
	case "":
		return "oMLX"
	case "splash":
		return "Splash"
	default:
		return engine
	}
}
