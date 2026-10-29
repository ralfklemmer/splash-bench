package appbundle

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// DMG packs the bundle at app into "<Name>-<Version>.dmg" in outDir and
// returns its path. The image holds the app and a link to /Applications,
// so installing is one drag.
func (b *Builder) DMG(ctx context.Context, app, outDir string) (string, error) {
	outDir, err := filepath.Abs(outDir)
	if err != nil {
		return "", fmt.Errorf("resolve %s: %w", outDir, err)
	}
	staging, err := os.MkdirTemp(outDir, ".dmg-*")
	if err != nil {
		return "", fmt.Errorf("create staging folder: %w", err)
	}
	defer os.RemoveAll(staging)

	// ditto keeps the signature and extended attributes intact.
	if err := b.run.Run(ctx, staging, "ditto", app, filepath.Join(staging, filepath.Base(app))); err != nil {
		return "", fmt.Errorf("copy app: %w", err)
	}
	if err := os.Symlink("/Applications", filepath.Join(staging, "Applications")); err != nil {
		return "", fmt.Errorf("link Applications: %w", err)
	}
	dmg := filepath.Join(outDir, DMGName(b.meta))
	if err := b.run.Run(ctx, outDir, "hdiutil", "create", "-volname", b.meta.Name, "-srcfolder", staging, "-ov", "-format", "UDZO", dmg); err != nil {
		return "", fmt.Errorf("create disk image: %w", err)
	}
	return dmg, nil
}

// DMGName is the file name of the disk image, without spaces so download
// links stay readable.
func DMGName(m Meta) string {
	return strings.ReplaceAll(m.Name, " ", "-") + "-" + m.Version + ".dmg"
}
