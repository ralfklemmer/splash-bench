package appbundle

import (
	"context"
	"fmt"
	"image"
	"image/png"
	"os"
	"path/filepath"
)

// CommandRunner runs a program in dir.
type CommandRunner interface {
	Run(ctx context.Context, dir, name string, args ...string) error
}

// Builder assembles the bundle from the Go module in moduleDir.
type Builder struct {
	run       CommandRunner
	moduleDir string
	meta      Meta
	// render draws the icon at one size; tests replace the slow renderer.
	render func(size int) image.Image
}

func NewBuilder(run CommandRunner, moduleDir string, meta Meta) *Builder {
	return &Builder{run: run, moduleDir: moduleDir, meta: meta, render: func(size int) image.Image { return Icon(size) }}
}

// Build writes "<Name>.app" into outDir and returns its path. It assembles
// the bundle next to the target first and replaces an older bundle only
// when everything succeeded.
func (b *Builder) Build(ctx context.Context, outDir string) (string, error) {
	// The tools run in other folders, so every path they get is absolute.
	outDir, err := filepath.Abs(outDir)
	if err != nil {
		return "", fmt.Errorf("resolve %s: %w", outDir, err)
	}
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return "", fmt.Errorf("create %s: %w", outDir, err)
	}
	staging, err := os.MkdirTemp(outDir, ".bundle-*")
	if err != nil {
		return "", fmt.Errorf("create staging folder: %w", err)
	}
	defer os.RemoveAll(staging)

	app := filepath.Join(staging, b.meta.Name+".app")
	if err := b.assemble(ctx, app, staging); err != nil {
		return "", err
	}
	target := filepath.Join(outDir, b.meta.Name+".app")
	if err := os.RemoveAll(target); err != nil {
		return "", fmt.Errorf("remove old %s: %w", target, err)
	}
	if err := os.Rename(app, target); err != nil {
		return "", fmt.Errorf("move bundle to %s: %w", target, err)
	}
	return target, nil
}

func (b *Builder) assemble(ctx context.Context, app, staging string) error {
	macOS := filepath.Join(app, "Contents", "MacOS")
	resources := filepath.Join(app, "Contents", "Resources")
	for _, dir := range []string{macOS, resources} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return fmt.Errorf("create %s: %w", dir, err)
		}
	}
	executable := filepath.Join(macOS, b.meta.Executable)
	// The app shows its version in the diagnosis users paste into issues.
	ldflags := "-X main.version=" + b.meta.Version
	if err := b.run.Run(ctx, b.moduleDir, "go", "build", "-trimpath", "-ldflags", ldflags, "-o", executable, "./cmd/splashui"); err != nil {
		return fmt.Errorf("build splashui: %w", err)
	}
	if err := os.WriteFile(filepath.Join(app, "Contents", "Info.plist"), InfoPlist(b.meta), 0o644); err != nil {
		return fmt.Errorf("write Info.plist: %w", err)
	}
	if err := b.icon(ctx, staging, filepath.Join(resources, "AppIcon.icns")); err != nil {
		return err
	}
	// Ad-hoc signature: enough to run on this Mac. Passing the app on needs
	// a Developer ID signature and notarization.
	if err := b.run.Run(ctx, staging, "codesign", "--force", "--sign", "-", app); err != nil {
		return fmt.Errorf("sign bundle: %w", err)
	}
	return nil
}

func (b *Builder) icon(ctx context.Context, staging, icns string) error {
	iconset := filepath.Join(staging, "AppIcon.iconset")
	if err := os.MkdirAll(iconset, 0o755); err != nil {
		return fmt.Errorf("create %s: %w", iconset, err)
	}
	for _, e := range Iconset {
		if err := writePNG(filepath.Join(iconset, e.Name), b.render(e.Size)); err != nil {
			return err
		}
	}
	if err := b.run.Run(ctx, staging, "iconutil", "--convert", "icns", "--output", icns, iconset); err != nil {
		return fmt.Errorf("convert icon: %w", err)
	}
	return nil
}

func writePNG(path string, img image.Image) error {
	f, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("create %s: %w", path, err)
	}
	if err := png.Encode(f, img); err != nil {
		f.Close()
		return fmt.Errorf("encode %s: %w", path, err)
	}
	if err := f.Close(); err != nil {
		return fmt.Errorf("close %s: %w", path, err)
	}
	return nil
}
