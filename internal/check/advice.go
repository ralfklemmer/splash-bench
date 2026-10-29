package check

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"path"
	"strings"
)

func (c *Checker) recommendationSection(ctx context.Context, f facts) (Section, []string) {
	s := Section{Title: "Empfehlung nach Arbeitsspeicher"}
	if !f.memoryOK {
		s.Findings = append(s.Findings, Finding{Status: StatusWarn, Text: "Ohne RAM-Angabe keine Empfehlung"})
		return s, nil
	}
	var commands []string
	fitting := 0
	for _, rec := range Recommend(f.memory, OMLXCatalog()) {
		finding := recommendationFinding(rec, f.memory)
		if rec.Fits() {
			fitting++
			downloads := c.downloadCommands(rec.Model, f.modelDirs)
			if rec.Model.Measured {
				commands = append(commands, downloads...)
			} else {
				finding.Hints = append(finding.Hints, downloads...)
			}
		}
		s.Findings = append(s.Findings, finding)
	}
	if fitting == 0 {
		s.Findings = append(s.Findings, Finding{Status: StatusFail, Text: "Kein Splash-Modell für oMLX passt in den Arbeitsspeicher dieses Macs"})
	}
	s.Findings = append(s.Findings, catalogFindings()...)
	if len(commands) > 0 {
		commands = append(c.hfInstallHint(ctx), commands...)
		commands = append(commands, "# danach oMLX neu starten (oder POST /admin/api/reload – entlädt alle Modelle)")
	}
	return s, commands
}

// recommendationFinding grades one model. A model that does not fit is only
// information: another one may fit, and recommendationSection fails when
// none does.
func recommendationFinding(rec Recommendation, mem Memory) Finding {
	m := rec.Model
	if !rec.Fits() {
		return Finding{Status: StatusInfo, Text: fmt.Sprintf("%s: passt nicht (braucht ≈ %s schon bei %s, nutzbar ≈ %s)",
			m.Family, formatRoundGB(m.MemoryGB(0)), ContextTiers[0].Label, formatRoundGB(mem.UsableGB()))}
	}
	tier := ContextTiers[rec.Tier]
	status := StatusOK
	if !m.Measured {
		status = StatusInfo
	}
	return Finding{Status: status,
		Text:  fmt.Sprintf("%s: bis %s Kontext (≈ %s mit Splash)", m.Family, tier.Label, formatRoundGB(m.MemoryGB(rec.Tier))),
		Hints: []string{m.Advice}}
}

// catalogFindings names the families Splash serves without oMLX and the
// date the catalog was checked.
func catalogFindings() []Finding {
	var names []string
	for _, m := range Catalog {
		if !m.InOMLX {
			names = append(names, m.Family)
		}
	}
	var findings []Finding
	if len(names) > 0 {
		findings = append(findings, Finding{Status: StatusInfo, Text: "Ohne oMLX (splash serve, LM Studio): " + strings.Join(names, ", ")})
	}
	return append(findings, Finding{Status: StatusInfo,
		Text: fmt.Sprintf("Katalog-Stand %s · Splash %s · oMLX %s", CatalogAsOf, CatalogSplash, CatalogOMLX)})
}

// downloadCommands returns `hf download` lines for missing or incomplete
// target and draft folders. The first model directory is the destination.
func (c *Checker) downloadCommands(m TargetModel, dirs []string) []string {
	if len(dirs) == 0 {
		return nil
	}
	var commands []string
	if LocateTarget(c.files, dirs, m).State != FolderComplete {
		commands = append(commands, HFDownload(m.Target, m.TargetFiles, ShellPath(path.Join(dirs[0], string(m.Target)), c.cfg.Home)))
	}
	if Locate(c.files, dirs, m.Draft).State != FolderComplete {
		commands = append(commands, HFDownload(m.Draft, nil, ShellPath(path.Join(dirs[0], string(m.Draft)), c.cfg.Home)))
	}
	return commands
}

func (c *Checker) hfInstallHint(ctx context.Context) []string {
	if _, err := c.commands.Output(ctx, "hf", "version"); errors.Is(err, exec.ErrNotFound) {
		return []string{"brew install huggingface-cli   # liefert den Befehl hf"}
	}
	return nil
}
