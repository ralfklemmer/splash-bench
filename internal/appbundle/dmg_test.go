package appbundle

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDMGName(t *testing.T) {
	tests := []struct {
		meta Meta
		want string
	}{
		{Meta{Name: "Splash Bench", Version: "1.2.0"}, "Splash-Bench-1.2.0.dmg"},
		{Meta{Name: "X", Version: "dev"}, "X-dev.dmg"},
	}
	for _, tt := range tests {
		if got := DMGName(tt.meta); got != tt.want {
			t.Errorf("DMGName(%+v) = %q, want %q", tt.meta, got, tt.want)
		}
	}
}

// dmgRecorder checks the staging folder when hdiutil runs, since it is
// gone afterwards.
type dmgRecorder struct {
	fakeRunner
	staged []string
	args   string
}

func (d *dmgRecorder) Run(ctx context.Context, dir, name string, args ...string) error {
	if name == "hdiutil" {
		d.args = strings.Join(args, " ")
		for i, a := range args {
			if a == "-srcfolder" {
				entries, _ := os.ReadDir(args[i+1])
				for _, e := range entries {
					d.staged = append(d.staged, e.Name())
				}
			}
		}
	}
	return d.fakeRunner.Run(ctx, dir, name, args...)
}

func TestDMGStagesAppAndApplicationsLink(t *testing.T) {
	out := t.TempDir()
	run := &dmgRecorder{}
	b := testBuilder(run)
	b.meta.Version = "1.2.0"
	dmg, err := b.DMG(context.Background(), "/x/Splash Bench.app", out)
	if err != nil {
		t.Fatalf("DMG: %v", err)
	}
	if dmg != filepath.Join(out, "Splash-Bench-1.2.0.dmg") {
		t.Errorf("dmg = %s", dmg)
	}
	if got := strings.Join(run.calls, " "); got != "ditto hdiutil" {
		t.Errorf("commands = %s", got)
	}
	// ditto is faked, so only the link is really there.
	if got := strings.Join(run.staged, " "); got != "Applications" {
		t.Errorf("staged = %q, want the Applications link", got)
	}
	if !strings.Contains(run.args, "-volname Splash Bench ") || !strings.HasSuffix(run.args, dmg) {
		t.Errorf("hdiutil args = %q", run.args)
	}
	if leftovers, _ := filepath.Glob(filepath.Join(out, ".dmg-*")); len(leftovers) != 0 {
		t.Errorf("staging left behind: %v", leftovers)
	}
}

func TestDMGReportsFailure(t *testing.T) {
	b := testBuilder(&fakeRunner{failOn: "hdiutil"})
	if _, err := b.DMG(context.Background(), "/x/Splash Bench.app", t.TempDir()); err == nil || !strings.Contains(err.Error(), "disk image") {
		t.Errorf("err = %v, want disk image failure", err)
	}
}
