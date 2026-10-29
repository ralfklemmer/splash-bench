// Package readiness checks, step by step, whether this Mac can run a
// comparison, and tells the user in plain German what is missing and what
// to do about it.
package readiness

import (
	"context"
	"errors"
	"fmt"
	"math"
	"strings"

	"github.com/ralfklemmer/splash-bench/internal/check"
)

// Level is the outcome of one check.
type Level string

const (
	LevelOK   Level = "ok"
	LevelWarn Level = "warn" // works, but with a limitation the user should know
	LevelFail Level = "fail" // the comparison cannot run
	LevelSkip Level = "skip" // not checked because an earlier check failed
)

// OMLXReleases is where new oMLX versions are published.
const OMLXReleases = "github.com/jundot/omlx/releases"

// Check is one requirement as the user sees it. Command is a ready-to-copy
// shell command, if one helps.
type Check struct {
	Title   string `json:"title"`
	Level   Level  `json:"level"`
	Detail  string `json:"detail"`
	Hint    string `json:"hint,omitempty"`
	Command string `json:"command,omitempty"`
}

// ModelChoice is a model the user can pick. NeedGB is the estimated memory
// with Splash (0 when unknown); Fits tells whether it fits the usable RAM.
type ModelChoice struct {
	ID          string  `json:"id"`
	Name        string  `json:"name"`
	Ready       bool    `json:"ready"`
	Reason      string  `json:"reason,omitempty"`
	Recommended bool    `json:"recommended,omitempty"`
	NeedGB      float64 `json:"need_gb,omitempty"`
	Fits        bool    `json:"fits"`
	// Command downloads the DFlash2 draft oMLX says the model lacks.
	Command string `json:"command,omitempty"`
}

// CatalogEntry is a catalog model as the user sees it: whether it fits
// and is ready, and the commands that download what oMLX does not list.
// InOMLX is false for families Splash serves only without oMLX; they carry
// no command.
type CatalogEntry struct {
	Name    string  `json:"name"`
	NeedGB  float64 `json:"need_gb"`
	Fits    bool    `json:"fits"`
	Ready   bool    `json:"ready"`
	InOMLX  bool    `json:"in_omlx"`
	Advice  string  `json:"advice"`
	Command string  `json:"command,omitempty"`
}

// Status is the readiness of this Mac. Ready is true when no check failed.
type Status struct {
	Ready     bool           `json:"ready"`
	Connected bool           `json:"connected"`
	BaseURL   string         `json:"base_url"`
	System    check.System   `json:"system"`
	Models    []ModelChoice  `json:"models"`
	Catalog   []CatalogEntry `json:"catalog"`
	// CatalogAsOf says when and against which releases the catalog was checked.
	CatalogAsOf string  `json:"catalog_as_of"`
	Checks      []Check `json:"checks"`
	// Diagnosis sums up this Mac and the checks as plain text to paste
	// into a bug report.
	Diagnosis string `json:"diagnosis"`
}

// Errors the Server adapter maps oMLX answers to.
var (
	ErrStarting     = errors.New("oMLX startet noch")
	ErrUnauthorized = errors.New("oMLX lehnt die Anfrage ohne gültigen API-Key ab")
)

// ServerModel is a model as oMLX lists it. Splash is nil when oMLX reports
// nothing about Splash for it (oMLX without external-engine support).
type ServerModel struct {
	ID        string
	SizeBytes int64
	Splash    *SplashSupport
}

// SplashSupport is oMLX's verdict whether Splash can serve a model.
// DraftRepo names the DFlash2 draft oMLX misses for it; oMLX only knows
// one for families it can hand to Splash.
type SplashSupport struct {
	Compatible bool
	Reason     string
	DraftRepo  string
}

// Server is the part of oMLX the checks need.
type Server interface {
	Health(ctx context.Context) error
	Models(ctx context.Context) ([]ServerModel, error)
}

// SystemReader reads the hardware and software profile.
type SystemReader interface {
	System(ctx context.Context) check.System
}

// Config describes how the app reaches oMLX.
type Config struct {
	BaseURL string
	// BaseURLSource says where BaseURL came from, for the hint.
	BaseURLSource string
	KeySet        bool
	// ModelDir is the oMLX model directory as a shell path, the destination
	// of the suggested downloads.
	ModelDir string
	// AppVersion is the version of Splash Bench, for the diagnosis.
	AppVersion string
}

// Checker runs the checks. It only reads; it never changes settings.
type Checker struct {
	server Server
	system SystemReader
	cfg    Config
}

func NewChecker(server Server, system SystemReader, cfg Config) *Checker {
	return &Checker{server: server, system: system, cfg: cfg}
}

// Status runs all checks in order. Server checks after a failed one are
// skipped, since they would fail for the same reason.
func (c *Checker) Status(ctx context.Context) Status {
	sys := c.system.System(ctx)
	st := Status{BaseURL: c.cfg.BaseURL, System: sys, Models: []ModelChoice{}}
	st.Catalog = catalog(nil, nil, sys, c.cfg.ModelDir)
	st.CatalogAsOf = fmt.Sprintf("Stand %s · Splash %s · oMLX %s", check.CatalogAsOf, check.CatalogSplash, check.CatalogOMLX)
	st.Checks = append(st.Checks, chipCheck(sys), macOSCheck(sys), splashCheck(sys))

	server := c.serverChecks(ctx, sys, &st)
	st.Checks = append(st.Checks, server...)

	st.Ready = true
	for _, ch := range st.Checks {
		if ch.Level == LevelFail {
			st.Ready = false
		}
	}
	st.Diagnosis = diagnosis(c.cfg.AppVersion, st)
	return st
}

// diagnosis renders the facts a bug report needs. It holds no secrets:
// the API key never reaches a Status.
func diagnosis(appVersion string, st Status) string {
	unknown := func(v string) string {
		if v == "" {
			return "unbekannt"
		}
		return v
	}
	sys := st.System
	var b strings.Builder
	fmt.Fprintf(&b, "Splash Bench: %s\n", unknown(appVersion))
	fmt.Fprintf(&b, "Chip: %s\n", unknown(sys.Chip))
	if sys.MemoryGB > 0 {
		fmt.Fprintf(&b, "Arbeitsspeicher: %s GB\n", gb(sys.MemoryGB))
	} else {
		b.WriteString("Arbeitsspeicher: unbekannt\n")
	}
	fmt.Fprintf(&b, "macOS: %s\n", unknown(sys.MacOS))
	launcher := sys.Launcher
	if launcher == "" {
		launcher = "splash"
	}
	fmt.Fprintf(&b, "Splash (%s): %s\n", launcher, unknown(sys.Splash))
	fmt.Fprintf(&b, "oMLX: %s\n", unknown(sys.OMLX))
	b.WriteString("\nVoraussetzungen:\n")
	marks := map[Level]string{LevelOK: "✓", LevelWarn: "!", LevelFail: "✗", LevelSkip: "–"}
	for _, ch := range st.Checks {
		fmt.Fprintf(&b, "%s %s: %s\n", marks[ch.Level], ch.Title, ch.Detail)
	}
	return b.String()
}

func (c *Checker) serverChecks(ctx context.Context, sys check.System, st *Status) []Check {
	titles := []string{"oMLX erreichbar", "Zugriff auf oMLX", "oMLX mit Splash-Unterstützung", "Splash-fähiges Modell", "Arbeitsspeicher"}
	skipRest := func(done []Check) []Check {
		for _, t := range titles[len(done):] {
			done = append(done, Check{Title: t, Level: LevelSkip, Detail: "nicht geprüft"})
		}
		return done
	}

	reach := c.reachCheck(ctx, sys)
	checks := []Check{reach}
	if reach.Level == LevelFail {
		return skipRest(checks)
	}
	st.Connected = true

	models, err := c.server.Models(ctx)
	access := c.accessCheck(err)
	checks = append(checks, access)
	if access.Level == LevelFail {
		return skipRest(checks)
	}

	support := supportCheck(models, sys)
	checks = append(checks, support)
	if support.Level == LevelFail {
		return skipRest(checks)
	}

	st.Models = choices(models, sys, c.cfg.ModelDir)
	st.Catalog = catalog(models, st.Models, sys, c.cfg.ModelDir)
	model := modelCheck(st.Models, models, sys, c.cfg.ModelDir)
	checks = append(checks, model)
	if model.Level == LevelFail {
		return skipRest(checks)
	}
	return append(checks, memoryCheck(st.Models, sys))
}

func chipCheck(sys check.System) Check {
	ch := Check{Title: "Chip"}
	chip := check.Chip{Brand: "Apple " + sys.Chip}
	switch {
	case sys.Chip == "":
		ch.Level, ch.Detail = LevelWarn, "Chip nicht lesbar – weiter mit Vorbehalt."
	case !sys.AppleSilicon:
		ch.Level, ch.Detail = LevelFail, sys.Chip
		ch.Hint = "Splash läuft nur auf Macs mit Apple Silicon, offiziell ab M3."
	case !chip.OfficialSplash():
		ch.Level, ch.Detail = LevelWarn, sys.Chip+" – Splash von Inco AI braucht M3 oder neuer."
		ch.Hint = "Auf M1/M2 geht nur der inoffizielle Community-Port splash-m1, ohne Support von Inco AI."
	default:
		ch.Level, ch.Detail = LevelOK, sys.Chip
	}
	return ch
}

// macOSCheck fails below the macOS the official Splash installs on. For the
// community port splash-m1 the requirement is unknown, so it only warns.
func macOSCheck(sys check.System) Check {
	ch := Check{Title: "macOS"}
	minimum := fmt.Sprintf("%d.%d", check.MinMacOS.Major, check.MinMacOS.Minor)
	v, err := check.ParseMacOSVersion(sys.MacOS)
	switch {
	case err != nil:
		ch.Level, ch.Detail = LevelWarn, "Version nicht lesbar – Splash braucht macOS "+minimum+" oder neuer."
	case !v.Less(check.MinMacOS):
		ch.Level, ch.Detail = LevelOK, sys.MacOS
	case sys.Launcher == "splash-m1":
		ch.Level, ch.Detail = LevelWarn, sys.MacOS+" – Splash von Inco AI braucht macOS "+minimum+"."
		ch.Hint = "Ob splash-m1 auf älterem macOS läuft, ist nicht geprüft."
	default:
		ch.Level, ch.Detail = LevelFail, sys.MacOS+" – Splash braucht macOS "+minimum+" oder neuer."
		ch.Hint = "macOS aktualisieren (Systemeinstellungen → Allgemein → Softwareupdate), danach erneut prüfen."
	}
	return ch
}

func splashCheck(sys check.System) Check {
	ch := Check{Title: "Splash installiert"}
	if sys.Splash == "" {
		ch.Level, ch.Detail = LevelFail, sys.Launcher+" nicht gefunden."
		if sys.Launcher == "splash-m1" {
			ch.Hint = "Auf M1/M2 den inoffiziellen Community-Port splash-m1 installieren (nicht von Inco AI): " + check.SplashM1Releases + " – danach erneut prüfen."
		} else {
			ch.Hint = "Splash mit Homebrew installieren, danach erneut prüfen."
			ch.Command = "brew install incoai/tap/splash"
		}
		return ch
	}
	v, err := check.ParseVersion(sys.Splash)
	switch {
	case err == nil && v.Less(check.MinSplashVersion):
		ch.Level = LevelFail
		ch.Detail = fmt.Sprintf("%s %s – oMLX braucht mindestens %s.", sys.Launcher, sys.Splash, check.MinSplashVersion)
		ch.Hint, ch.Command = upgradeAdvice(sys.Launcher)
	case err == nil && v.Less(check.FlashNextVersion):
		ch.Level = LevelWarn
		ch.Detail = fmt.Sprintf("%s %s – Qwen3.8-Flash-Next braucht %s.", sys.Launcher, sys.Splash, check.FlashNextVersion)
		ch.Hint, ch.Command = upgradeAdvice(sys.Launcher)
	default:
		ch.Level, ch.Detail = LevelOK, splashVersion(sys)
	}
	return ch
}

// splashVersion names the installed version, and the launcher when it is
// the community port.
func splashVersion(sys check.System) string {
	if sys.Launcher == "splash-m1" {
		return "Version " + sys.Splash + " (splash-m1)"
	}
	return "Version " + sys.Splash
}

// omlxVersion is the installed oMLX app version for the user.
func omlxVersion(sys check.System) string {
	if sys.OMLX == "" {
		return "oMLX-App nicht gefunden"
	}
	return "Version " + sys.OMLX
}

// upgradeAdvice says how to update the launcher.
func upgradeAdvice(launcher string) (hint, command string) {
	if launcher == "splash-m1" {
		return "Neue Version von " + check.SplashM1Releases + " installieren.", ""
	}
	return "Splash aktualisieren, danach oMLX neu starten.", "brew update && brew upgrade incoai/tap/splash"
}

func (c *Checker) reachCheck(ctx context.Context, sys check.System) Check {
	ch := Check{Title: "oMLX erreichbar"}
	err := c.server.Health(ctx)
	switch {
	case errors.Is(err, ErrStarting):
		ch.Level, ch.Detail = LevelFail, "oMLX startet noch und lädt Modelle."
		ch.Hint = "Kurz warten, dann erneut prüfen."
	case err != nil:
		ch.Level = LevelFail
		ch.Detail = omlxVersion(sys) + " · unter " + c.cfg.BaseURL + " antwortet kein oMLX."
		ch.Hint = "oMLX starten. Die Adresse kommt aus " + c.cfg.BaseURLSource + "."
		if sys.OMLX == "" {
			ch.Hint = "oMLX installieren (" + OMLXReleases + ") und starten – oder, falls oMLX anders installiert ist, starten. Die Adresse kommt aus " + c.cfg.BaseURLSource + "."
		}
	default:
		ch.Level, ch.Detail = LevelOK, omlxVersion(sys)+" · "+c.cfg.BaseURL
		if sys.OMLX == "" {
			ch.Detail = "läuft, Version unbekannt · " + c.cfg.BaseURL
		}
	}
	return ch
}

func (c *Checker) accessCheck(err error) Check {
	ch := Check{Title: "Zugriff auf oMLX"}
	switch {
	case errors.Is(err, ErrUnauthorized) && c.cfg.KeySet:
		ch.Level, ch.Detail = LevelFail, "oMLX lehnt den API-Key ab."
		ch.Hint = "auth.api_key in ~/.omlx/settings.json prüfen oder OMLX_API_KEY passend setzen."
	case errors.Is(err, ErrUnauthorized):
		ch.Level, ch.Detail = LevelFail, "oMLX verlangt einen API-Key, es ist aber keiner hinterlegt."
		ch.Hint = "In oMLX einen API-Key festlegen (steht dann in ~/.omlx/settings.json) oder OMLX_API_KEY setzen."
	case err != nil:
		ch.Level, ch.Detail = LevelFail, "Modellliste nicht lesbar: "+err.Error()
	case c.cfg.KeySet:
		ch.Level, ch.Detail = LevelOK, "mit API-Key"
	default:
		ch.Level, ch.Detail = LevelOK, "ohne API-Key (oMLX erlaubt lokale Anfragen)"
	}
	return ch
}

// supportCheck fails when oMLX reports Splash information for no model:
// such a version cannot hand models to an external engine at all.
func supportCheck(models []ServerModel, sys check.System) Check {
	ch := Check{Title: "oMLX mit Splash-Unterstützung"}
	for _, m := range models {
		if m.Splash != nil {
			ch.Level, ch.Detail = LevelOK, "externe Engine verfügbar"
			return ch
		}
	}
	if len(models) == 0 {
		ch.Level, ch.Detail = LevelOK, "keine Modelle zum Prüfen"
		return ch
	}
	version := sys.OMLX
	if version == "" {
		version = "unbekannt"
	}
	ch.Level = LevelFail
	ch.Detail = "Diese oMLX-Version kennt keine externe Engine (installiert: " + version + ")."
	ch.Hint = "oMLX auf die aktuelle Version bringen: " + OMLXReleases + "."
	return ch
}

func modelCheck(choices []ModelChoice, models []ServerModel, sys check.System, modelDir string) Check {
	ch := Check{Title: "Splash-fähiges Modell"}
	var ready, blocked, drafts []string
	for _, m := range choices {
		switch {
		case m.Ready:
			ready = append(ready, m.Name)
		default:
			blocked = append(blocked, m.Name+": "+m.Reason)
			if m.Command != "" {
				drafts = append(drafts, m.Command)
			}
		}
	}
	if len(drafts) > 0 {
		ch.Hint = "oMLX nennt den fehlenden DFlash2-Draft. Herunterladen, danach oMLX neu starten und erneut prüfen."
		ch.Command = strings.Join(drafts, "\n")
	}
	if len(ready) > 0 {
		ch.Level, ch.Detail = LevelOK, strings.Join(ready, ", ")
		if len(blocked) > 0 {
			ch.Level = LevelWarn
			ch.Detail += " · nicht bereit: " + strings.Join(blocked, "; ")
		}
		return ch
	}
	ch.Level = LevelFail
	ch.Detail = "Kein Modell, das Splash bedienen kann."
	if len(blocked) > 0 {
		ch.Detail += " " + strings.Join(blocked, "; ")
	}
	if len(drafts) == 0 {
		ch.Hint, ch.Command = modelAdvice(models, sys, modelDir)
	}
	return ch
}

// modelAdvice names the catalog model that suits this Mac's memory and the
// commands that download what oMLX does not list yet. Without a readable
// memory size it suggests the first catalog model.
func modelAdvice(models []ServerModel, sys check.System, modelDir string) (hint, command string) {
	target, ok := suitableModel(sys)
	if !ok {
		catalog := check.OMLXCatalog()
		smallest := catalog[0].MemoryGB(0)
		for _, t := range catalog[1:] {
			smallest = math.Min(smallest, t.MemoryGB(0))
		}
		return fmt.Sprintf("Für die Splash-Modelle reicht der Arbeitsspeicher nicht: das kleinste braucht ca. %s GB, nutzbar sind ca. %s GB.",
			gb(smallest), gb(sys.UsableGB())), ""
	}
	hint = fmt.Sprintf("Passend für diesen Mac: %s (ca. %s GB mit Splash). Zielmodell und DFlash2-Draft herunterladen – zusammen ca. %s GB, braucht das Programm hf (brew install huggingface-cli) –, danach oMLX neu starten und erneut prüfen.",
		target.Family, gb(target.MemoryGB(0)), gb(target.TargetGB+target.DraftGB))
	return hint, downloadCommand(target, models, modelDir)
}

// downloadCommand lists one `hf download` line for target and draft each
// that oMLX does not list yet; empty when it lists both.
func downloadCommand(target check.TargetModel, models []ServerModel, modelDir string) string {
	listed := map[string]bool{}
	for _, m := range models {
		listed[m.ID] = true
	}
	var lines []string
	if !listed[target.TargetID()] {
		lines = append(lines, hfDownload(target.Target, target.TargetFiles, modelDir))
	}
	if !listed[target.Draft.Name()] {
		lines = append(lines, hfDownload(target.Draft, nil, modelDir))
	}
	return strings.Join(lines, "\n")
}

func hfDownload(repo check.Repo, files []string, modelDir string) string {
	return check.HFDownload(repo, files, modelDir+"/"+repo.Name())
}

// catalog describes every catalog model for this Mac. A model is ready
// when oMLX offers it for Splash; then no download is suggested. Families
// Splash serves only without oMLX get no command, and a model needing a
// newer Splash says so.
func catalog(models []ServerModel, choices []ModelChoice, sys check.System, modelDir string) []CatalogEntry {
	installed, versionErr := check.ParseVersion(sys.Splash)
	out := make([]CatalogEntry, 0, len(check.Catalog))
	for _, t := range check.Catalog {
		e := CatalogEntry{Name: t.Family, NeedGB: t.MemoryGB(0), Advice: t.Advice, InOMLX: t.InOMLX}
		e.Fits = sys.MemoryGB == 0 || e.NeedGB <= sys.UsableGB()
		if versionErr == nil && installed.Less(t.RequiredSplash()) {
			e.Advice += fmt.Sprintf(" Braucht Splash %s (installiert: %s).", t.RequiredSplash(), sys.Splash)
		}
		for _, mc := range choices {
			if mc.ID == t.TargetID() && mc.Ready {
				e.Ready = true
			}
		}
		if !e.Ready && t.InOMLX {
			e.Command = downloadCommand(withServerDraft(t, models), models, modelDir)
		}
		out = append(out, e)
	}
	return out
}

// withServerDraft prefers the draft oMLX names for the model over the
// catalog's, so a renamed draft needs no catalog update.
func withServerDraft(t check.TargetModel, models []ServerModel) check.TargetModel {
	for _, m := range models {
		if m.ID == t.TargetID() && m.Splash != nil && m.Splash.DraftRepo != "" {
			t.Draft = check.Repo(m.Splash.DraftRepo)
		}
	}
	return t
}

// suitableModel is the first catalog model that fits the usable memory at
// the smallest context.
func suitableModel(sys check.System) (check.TargetModel, bool) {
	if sys.MemoryGB == 0 {
		return check.OMLXCatalog()[0], true
	}
	for _, rec := range check.Recommend(check.Memory{Bytes: uint64(sys.MemoryGB * (1 << 30))}, check.OMLXCatalog()) {
		if rec.Fits() {
			return rec.Model, true
		}
	}
	return check.TargetModel{}, false
}

// memoryCheck compares the estimated need of the selectable models with
// the RAM the GPU may use.
func memoryCheck(choices []ModelChoice, sys check.System) Check {
	ch := Check{Title: "Arbeitsspeicher"}
	if sys.MemoryGB == 0 {
		ch.Level, ch.Detail = LevelWarn, "nicht lesbar – ob die Modelle passen, zeigt erst die Messung."
		return ch
	}
	usable := fmt.Sprintf("%s GB, davon ca. %s GB für Modelle", gb(sys.MemoryGB), gb(sys.UsableGB()))
	var tight []string
	fitting := 0
	for _, m := range choices {
		if !m.Ready {
			continue
		}
		if m.Fits {
			fitting++
		} else {
			tight = append(tight, fmt.Sprintf("%s braucht ca. %s GB", m.Name, gb(m.NeedGB)))
		}
	}
	switch {
	case len(tight) == 0:
		ch.Level, ch.Detail = LevelOK, usable
	case fitting > 0:
		ch.Level, ch.Detail = LevelWarn, usable+" · "+strings.Join(tight, ", ")
		ch.Hint = "Diese Modelle sind nicht vorausgewählt; beim Laden kann der Vergleich abbrechen."
	default:
		ch.Level, ch.Detail = LevelFail, usable+" · "+strings.Join(tight, ", ")
		ch.Hint = "Kein Splash-fähiges Modell passt in den Arbeitsspeicher dieses Macs."
	}
	return ch
}

// choices keeps the models Splash can serve, plus those it cannot serve
// yet but oMLX or the catalog knows (so their reason and the missing draft
// show), with the memory estimate.
func choices(models []ServerModel, sys check.System, modelDir string) []ModelChoice {
	out := []ModelChoice{}
	for _, m := range models {
		target, inCatalog := catalogModel(m.ID)
		if m.Splash == nil || !(m.Splash.Compatible || inCatalog || m.Splash.DraftRepo != "") {
			continue
		}
		mc := ModelChoice{ID: m.ID, Name: m.ID, Ready: m.Splash.Compatible, Reason: m.Splash.Reason, Recommended: inCatalog}
		switch {
		case inCatalog:
			mc.Name = target.Family
			mc.NeedGB = target.MemoryGB(0)
		case m.SizeBytes > 0:
			mc.NeedGB = float64(m.SizeBytes)/(1<<30) + check.SplashOverheadGB
		}
		if !mc.Ready && m.Splash.DraftRepo != "" {
			mc.Command = hfDownload(check.Repo(m.Splash.DraftRepo), nil, modelDir)
		}
		mc.Fits = sys.MemoryGB == 0 || mc.NeedGB == 0 || mc.NeedGB <= sys.UsableGB()
		out = append(out, mc)
	}
	return out
}

func catalogModel(id string) (check.TargetModel, bool) {
	for _, t := range check.OMLXCatalog() {
		if t.TargetID() == id {
			return t, true
		}
	}
	return check.TargetModel{}, false
}

func gb(v float64) string {
	return strings.Replace(fmt.Sprintf("%.0f", math.Round(v)), ".", ",", 1)
}
