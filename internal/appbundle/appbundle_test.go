package appbundle

import (
	"context"
	"errors"
	"image"
	"image/color"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestInfoPlist(t *testing.T) {
	plist := string(InfoPlist(Meta{Name: "Splash & Bench", Identifier: "de.example.x", Version: "2.1", Executable: "splashui"}))
	for _, want := range []string{
		"<key>CFBundleExecutable</key>\n\t<string>splashui</string>",
		"<key>CFBundleIdentifier</key>\n\t<string>de.example.x</string>",
		"<key>CFBundleShortVersionString</key>\n\t<string>2.1</string>",
		"<key>CFBundleIconFile</key>\n\t<string>AppIcon</string>",
		"<key>LSUIElement</key>\n\t<true/>",
		"<string>Splash &amp; Bench</string>",
	} {
		if !strings.Contains(plist, want) {
			t.Errorf("Info.plist misses %q:\n%s", want, plist)
		}
	}
	if !strings.HasPrefix(plist, "<?xml") || !strings.HasSuffix(plist, "</plist>\n") {
		t.Errorf("Info.plist is not a complete document")
	}
}

func TestIcon(t *testing.T) {
	img := Icon(1024)
	tests := []struct {
		name string
		x, y int
		want color.RGBA
	}{
		{"transparent corner", 10, 10, color.RGBA{}},
		{"tile background", 200, 860, tileColor},
		{"oMLX bar", 256 + 6*17, 650, omlxColor},
		{"tallest Splash bar", 256 + 24*17, 400, splashTone},
	}
	for _, tt := range tests {
		if got := img.RGBAAt(tt.x, tt.y); got != tt.want {
			t.Errorf("%s at (%d,%d) = %v, want %v", tt.name, tt.x, tt.y, got, tt.want)
		}
	}
	if b := Icon(16).Bounds(); b.Dx() != 16 || b.Dy() != 16 {
		t.Errorf("small icon bounds = %v", b)
	}
}

// fakeRunner records commands and creates the files the real tools write.
type fakeRunner struct {
	calls  []string
	failOn string
}

func (f *fakeRunner) Run(_ context.Context, _ string, name string, args ...string) error {
	f.calls = append(f.calls, name)
	if name == f.failOn {
		return errors.New(name + " failed")
	}
	switch name {
	case "go":
		return os.WriteFile(args[len(args)-2], []byte("binary"), 0o755) // -o <file> ./cmd/splashui
	case "iconutil":
		return os.WriteFile(args[len(args)-2], []byte("icns"), 0o644) // --output <file> <iconset>
	}
	return nil
}

func testBuilder(run CommandRunner) *Builder {
	b := NewBuilder(run, ".", DefaultMeta)
	b.render = func(size int) image.Image { return image.NewRGBA(image.Rect(0, 0, size, size)) }
	return b
}

func TestBuildAssemblesBundle(t *testing.T) {
	out := t.TempDir()
	run := &fakeRunner{}
	app, err := testBuilder(run).Build(context.Background(), out)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if app != filepath.Join(out, "Splash Bench.app") {
		t.Errorf("app = %s", app)
	}
	for _, f := range []string{"Contents/MacOS/splashui", "Contents/Info.plist", "Contents/Resources/AppIcon.icns"} {
		if _, err := os.Stat(filepath.Join(app, f)); err != nil {
			t.Errorf("missing %s: %v", f, err)
		}
	}
	if got := strings.Join(run.calls, " "); got != "go iconutil codesign" {
		t.Errorf("commands = %s", got)
	}
	if leftovers, _ := filepath.Glob(filepath.Join(out, ".bundle-*")); len(leftovers) != 0 {
		t.Errorf("staging left behind: %v", leftovers)
	}
}

func TestBuildPassesAbsolutePaths(t *testing.T) {
	t.Chdir(t.TempDir())
	run := &pathRecorder{}
	if _, err := testBuilder(run).Build(context.Background(), "bin"); err != nil {
		t.Fatalf("Build: %v", err)
	}
	for _, p := range run.paths {
		if !filepath.IsAbs(p) {
			t.Errorf("relative path %q handed to a tool", p)
		}
	}
}

func TestBuildStampsVersion(t *testing.T) {
	run := &argsRecorder{}
	meta := DefaultMeta
	meta.Version = "1.2.3"
	b := testBuilder(run)
	b.meta = meta
	if _, err := b.Build(context.Background(), t.TempDir()); err != nil {
		t.Fatalf("Build: %v", err)
	}
	if !strings.Contains(run.goArgs, "-ldflags -X main.version=1.2.3 ") {
		t.Errorf("go build args = %q, want the version stamped", run.goArgs)
	}
}

// argsRecorder is a fakeRunner that keeps the arguments of go build.
type argsRecorder struct {
	fakeRunner
	goArgs string
}

func (a *argsRecorder) Run(ctx context.Context, dir, name string, args ...string) error {
	if name == "go" {
		a.goArgs = strings.Join(args, " ")
	}
	return a.fakeRunner.Run(ctx, dir, name, args...)
}

// pathRecorder is a fakeRunner that also keeps every path-like argument.
type pathRecorder struct {
	fakeRunner
	paths []string
}

func (p *pathRecorder) Run(ctx context.Context, dir, name string, args ...string) error {
	for _, a := range args {
		if strings.Contains(a, "Splash Bench.app") || strings.HasSuffix(a, ".iconset") {
			p.paths = append(p.paths, a)
		}
	}
	return p.fakeRunner.Run(ctx, dir, name, args...)
}

func TestFailedBuildKeepsOldBundle(t *testing.T) {
	for _, step := range []string{"go", "iconutil", "codesign"} {
		t.Run(step, func(t *testing.T) {
			out := t.TempDir()
			old := filepath.Join(out, "Splash Bench.app", "Contents", "Info.plist")
			if err := os.MkdirAll(filepath.Dir(old), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(old, []byte("old"), 0o644); err != nil {
				t.Fatal(err)
			}

			_, err := testBuilder(&fakeRunner{failOn: step}).Build(context.Background(), out)
			if err == nil || !strings.Contains(err.Error(), step+" failed") {
				t.Fatalf("err = %v, want the %s failure", err, step)
			}
			if data, _ := os.ReadFile(old); string(data) != "old" {
				t.Error("old bundle was replaced by a broken one")
			}
			if leftovers, _ := filepath.Glob(filepath.Join(out, ".bundle-*")); len(leftovers) != 0 {
				t.Errorf("staging left behind: %v", leftovers)
			}
		})
	}
}
