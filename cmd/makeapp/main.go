// Command makeapp builds "Splash Bench.app" from this module:
//
//	go run ./cmd/makeapp                       # → bin/Splash Bench.app
//	go run ./cmd/makeapp -version 1.2.0 -dmg   # also bin/Splash-Bench-1.2.0.dmg
//
// Run it from the module root. The bundle is signed ad hoc, which is
// enough for this Mac; to pass it on it needs a Developer ID signature and
// notarization.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/exec"

	"github.com/ralfklemmer/splash-bench/internal/appbundle"
)

// execRunner runs programs and passes their output through.
type execRunner struct{}

func (execRunner) Run(ctx context.Context, dir, name string, args ...string) error {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Dir = dir
	cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("%s: %w", name, err)
	}
	return nil
}

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "Fehler:", err)
		os.Exit(1)
	}
}

func run() error {
	out := flag.String("out", "bin", "Zielordner für das App-Bundle")
	version := flag.String("version", appbundle.DefaultMeta.Version, "Versionsnummer der App")
	dmg := flag.Bool("dmg", false, "zusätzlich ein DMG zum Weitergeben bauen")
	flag.Parse()

	if _, err := os.Stat("go.mod"); errors.Is(err, os.ErrNotExist) {
		return errors.New("bitte im Hauptordner des Projekts ausführen (go.mod fehlt)")
	}
	meta := appbundle.DefaultMeta
	meta.Version = *version
	builder := appbundle.NewBuilder(execRunner{}, ".", meta)
	app, err := builder.Build(context.Background(), *out)
	if err != nil {
		return err
	}
	fmt.Println("Fertig:", app)
	if *dmg {
		image, err := builder.DMG(context.Background(), app, *out)
		if err != nil {
			return err
		}
		fmt.Println("DMG:", image)
	}
	fmt.Println("Zum Installieren in den Ordner „Programme“ ziehen oder direkt per Doppelklick starten.")
	return nil
}
