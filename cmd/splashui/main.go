// Command splashui is the graphical Splash Bench: a local web app that
// compares oMLX's own engine with Splash on the chosen models and tasks,
// shows the race live and keeps every result. It only listens on localhost
// and only talks to the local oMLX server.
//
// Started from "Splash Bench.app" it logs to ~/Library/Logs and quits by
// itself when no page has been open for a while; a second start opens the
// running instance instead of a new one.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/ralfklemmer/splash-bench/internal/bench"
	"github.com/ralfklemmer/splash-bench/internal/check"
	"github.com/ralfklemmer/splash-bench/internal/compare"
	"github.com/ralfklemmer/splash-bench/internal/omlx"
	"github.com/ralfklemmer/splash-bench/internal/readiness"
	"github.com/ralfklemmer/splash-bench/internal/store"
	"github.com/ralfklemmer/splash-bench/internal/web"
)

const defaultAddr = "127.0.0.1:8765"

// version is stamped by makeapp (-ldflags "-X main.version=…").
var version = "dev"

type systemClock struct{}

func (systemClock) Now() time.Time { return time.Now() }

// systemReader reads the hardware profile with local commands.
type systemReader struct{}

func (systemReader) System(ctx context.Context) check.System {
	return check.ReadSystem(ctx, check.ExecRunner{})
}

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "Fehler:", err)
		os.Exit(1)
	}
}

func run() error {
	executable, _ := os.Executable()
	bundled := insideAppBundle(executable)
	defaultIdle := time.Duration(0)
	if bundled {
		defaultIdle = 2 * time.Minute
		if err := logToFile(); err != nil {
			return err
		}
	}
	if err := os.Setenv("PATH", withToolDirs(os.Getenv("PATH"))); err != nil {
		return fmt.Errorf("set PATH: %w", err)
	}

	defaultDir, err := defaultResultsDir()
	if err != nil {
		return err
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("find home directory: %w", err)
	}
	settings, _ := readOMLXSettings(home, os.DirFS("/"))
	baseURL := flag.String("base-url", settings.baseURL(), "oMLX-Server (Standard aus "+settingsLabel+")")
	addr := flag.String("addr", defaultAddr, "Adresse der App (nur localhost)")
	resultsDir := flag.String("results", defaultDir, "Ordner für gespeicherte Messungen")
	load := flag.String("load", "", "Messung aus dieser Datei als „letzte Messung“ anbieten (auch results.json von splashbench)")
	noOpen := flag.Bool("no-open", false, "Browser nicht automatisch öffnen")
	idleQuit := flag.Duration("idle-quit", defaultIdle, "beenden, wenn so lange keine Seite offen ist (0 = nie)")
	flag.Parse()

	listener, err := listenLoopback(*addr)
	if err != nil {
		probe, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		if runningInstance(probe, http.DefaultClient, *addr) {
			fmt.Println("Splash Bench läuft bereits auf http://" + *addr)
			openBrowser("http://"+*addr, *noOpen)
			return nil
		}
		if listener, err = anyFreePort(*addr); err != nil {
			return err
		}
	}

	apiKey := settings.apiKey(os.Getenv)
	baseURLSource := settingsLabel
	if flagSet("base-url") {
		baseURLSource = "dem Aufruf (-base-url)"
	}
	client := omlx.NewClient(*baseURL, apiKey, &http.Client{Timeout: 30 * time.Minute})
	statusClient := omlx.NewClient(*baseURL, apiKey, &http.Client{Timeout: 5 * time.Second})
	results := store.NewDir(*resultsDir)
	session := compare.NewSession(bench.NewRunner(client, client, systemClock{}), results, systemReader{}, systemClock{})
	if err := rememberPrevious(session, results, *load); err != nil {
		return err
	}
	status := readiness.NewChecker(readinessServer{statusClient}, systemReader{}, readiness.Config{
		BaseURL: *baseURL, BaseURLSource: baseURLSource, KeySet: apiKey != "",
		ModelDir: modelDir(home), AppVersion: version,
	})

	appCtx, stopApp := context.WithCancel(context.Background())
	defer stopApp()
	handler := web.NewServer(session, status)
	server := &http.Server{
		Handler:           handler,
		ReadHeaderTimeout: 5 * time.Second,
		BaseContext:       func(net.Listener) context.Context { return appCtx },
	}
	served := make(chan error, 1)
	go func() { served <- server.Serve(listener) }()

	url := "http://" + listener.Addr().String()
	fmt.Printf("%s Splash Bench läuft auf %s – beenden mit Ctrl-C oder dem Knopf in der App\n", time.Now().Format(time.DateTime), url)
	fmt.Printf("Messungen werden gespeichert in %s\n", *resultsDir)
	openBrowser(url, *noOpen)

	// Keep the signal handler installed until the end: a second Ctrl-C must
	// not kill the program while it restores the engine.
	signals, stopSignals := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stopSignals()
	serveErr := waitForEnd(signals, served, handler, session, *idleQuit)

	if session.View().Phase == compare.PhaseRunning {
		fmt.Println("Breche den Vergleich ab und stelle die Engine-Einstellung wieder her …")
	}
	session.Cancel()
	session.Wait()
	stopApp()
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := server.Shutdown(shutdownCtx); err != nil && serveErr == nil {
		serveErr = fmt.Errorf("stop server: %w", err)
	}
	fmt.Println(time.Now().Format(time.DateTime), "Splash Bench beendet")
	if errors.Is(serveErr, http.ErrServerClosed) {
		return nil
	}
	return serveErr
}

// waitForEnd blocks until a signal, the quit button, the server failing or
// the idle timeout; it returns the server's error, if any.
func waitForEnd(signals context.Context, served <-chan error, handler *web.Server, session *compare.Session, idleQuit time.Duration) error {
	idle := idleTimer{timeout: idleQuit}
	tick := time.NewTicker(5 * time.Second)
	defer tick.Stop()
	for {
		select {
		case <-signals.Done():
			return nil
		case <-handler.Quit():
			return nil
		case err := <-served:
			return err
		case now := <-tick.C:
			noPage := handler.Clients() == 0 && session.View().Phase != compare.PhaseRunning
			if idle.expired(now, noPage) {
				fmt.Println("Keine Seite mehr offen – beende Splash Bench")
				return nil
			}
		}
	}
}

// flagSet reports whether the flag was given on the command line.
func flagSet(name string) bool {
	set := false
	flag.Visit(func(f *flag.Flag) { set = set || f.Name == name })
	return set
}

func openBrowser(url string, disabled bool) {
	if disabled {
		return
	}
	if err := exec.Command("open", url).Start(); err != nil {
		fmt.Fprintln(os.Stderr, "Browser nicht geöffnet:", err)
	}
}

// logToFile sends all output to ~/Library/Logs/Splash Bench.log, because a
// double-clicked app has no terminal.
func logToFile() error {
	home, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("find home directory: %w", err)
	}
	dir := filepath.Join(home, "Library", "Logs")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create %s: %w", dir, err)
	}
	f, err := os.OpenFile(filepath.Join(dir, "Splash Bench.log"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return fmt.Errorf("open log: %w", err)
	}
	os.Stdout, os.Stderr = f, f
	return nil
}

func defaultResultsDir() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("find config directory: %w", err)
	}
	return filepath.Join(dir, "splashbench", "results"), nil
}

// rememberPrevious offers the file from -load, or else the newest stored
// result, as the last measurement.
func rememberPrevious(session *compare.Session, results *store.Dir, load string) error {
	if load != "" {
		r, err := store.ReadFile(load)
		if err != nil {
			return err
		}
		session.Remember(r)
		return nil
	}
	r, ok, err := results.Latest()
	if err != nil {
		fmt.Fprintln(os.Stderr, "Letzte Messung nicht lesbar:", err)
		return nil
	}
	if ok {
		session.Remember(r)
	}
	return nil
}
