package check

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"path"
	"strings"
)

// The kit reads the real file system through an fs.FS rooted at "/", so
// tests can use fstest.MapFS. Paths in this package are absolute.
func fsPath(abs string) string {
	clean := strings.TrimPrefix(path.Clean(abs), "/")
	if clean == "" {
		return "."
	}
	return clean
}

// SettingsPath is the oMLX settings file below the home directory.
func SettingsPath(home string) string {
	return path.Join(home, ".omlx", "settings.json")
}

// DefaultModelDir is used when the settings name no model directory.
func DefaultModelDir(home string) string {
	return path.Join(home, ".omlx", "models")
}

type omlxSettings struct {
	Model struct {
		ModelDirs []string `json:"model_dirs"`
		ModelDir  string   `json:"model_dir"`
	} `json:"model"`
}

// ReadModelDirs returns the model directories from ~/.omlx/settings.json
// (model.model_dirs, falling back to model.model_dir). It never exposes
// other settings such as the API key.
func ReadModelDirs(fsys fs.FS, home string) ([]string, error) {
	data, err := fs.ReadFile(fsys, fsPath(SettingsPath(home)))
	if err != nil {
		return nil, fmt.Errorf("read oMLX settings: %w", err)
	}
	var s omlxSettings
	if err := json.Unmarshal(data, &s); err != nil {
		return nil, fmt.Errorf("parse oMLX settings: %w", err)
	}
	dirs := s.Model.ModelDirs
	if len(dirs) == 0 && s.Model.ModelDir != "" {
		dirs = []string{s.Model.ModelDir}
	}
	for i, d := range dirs {
		dirs[i] = expandHome(d, home)
	}
	return dirs, nil
}

func expandHome(p, home string) string {
	if p == "~" {
		return home
	}
	if rest, ok := strings.CutPrefix(p, "~/"); ok {
		return path.Join(home, rest)
	}
	return p
}

// FolderState tells whether a model folder is usable by oMLX.
type FolderState int

const (
	FolderMissing FolderState = iota
	// FolderIncomplete exists but has no config.json, so oMLX skips it.
	FolderIncomplete
	FolderComplete
)

// Location is where a repository was found.
type Location struct {
	Path  string
	State FolderState
}

// Locate searches the model directories for repo, both as <dir>/<org>/<name>
// and as <dir>/<name>. A complete folder wins over an incomplete one.
func Locate(fsys fs.FS, dirs []string, repo Repo) Location {
	found := Location{State: FolderMissing}
	for _, dir := range dirs {
		for _, candidate := range []string{path.Join(dir, string(repo)), path.Join(dir, repo.Name())} {
			state := folderState(fsys, candidate)
			if state > found.State {
				found = Location{Path: candidate, State: state}
			}
		}
	}
	return found
}

// LocateTarget finds the model's target. A download of single files (a GGUF
// variant) has no config.json; its folder is complete when it holds them all.
func LocateTarget(fsys fs.FS, dirs []string, m TargetModel) Location {
	if len(m.TargetFiles) == 0 {
		return Locate(fsys, dirs, m.Target)
	}
	found := Location{State: FolderMissing}
	for _, dir := range dirs {
		for _, candidate := range []string{path.Join(dir, string(m.Target)), path.Join(dir, m.Target.Name())} {
			state := filesState(fsys, candidate, m.TargetFiles)
			if state > found.State {
				found = Location{Path: candidate, State: state}
			}
		}
	}
	return found
}

func filesState(fsys fs.FS, dir string, files []string) FolderState {
	if !DirExists(fsys, dir) {
		return FolderMissing
	}
	for _, f := range files {
		if _, err := fs.Stat(fsys, fsPath(path.Join(dir, f))); err != nil {
			return FolderIncomplete
		}
	}
	return FolderComplete
}

func folderState(fsys fs.FS, dir string) FolderState {
	info, err := fs.Stat(fsys, fsPath(dir))
	if err != nil || !info.IsDir() {
		return FolderMissing
	}
	if _, err := fs.Stat(fsys, fsPath(path.Join(dir, "config.json"))); err != nil {
		return FolderIncomplete
	}
	return FolderComplete
}

// DirExists reports whether dir is an existing directory.
func DirExists(fsys fs.FS, dir string) bool {
	info, err := fs.Stat(fsys, fsPath(dir))
	return err == nil && info.IsDir()
}

// DirSizeGB sums the file sizes below dir in decimal GB.
func DirSizeGB(fsys fs.FS, dir string) (float64, error) {
	var total int64
	err := fs.WalkDir(fsys, fsPath(dir), func(_ string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		total += info.Size()
		return nil
	})
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return 0, fmt.Errorf("measure %s: %w", dir, err)
	}
	return float64(total) / 1e9, nil
}

// ShellPath shortens a path below home to ~/… for display and commands;
// paths with spaces are quoted with $HOME so they can be pasted as-is.
func ShellPath(p, home string) string {
	rest, below := strings.CutPrefix(p, home+"/")
	switch {
	case below && !strings.ContainsAny(rest, " '\""):
		return "~/" + rest
	case below:
		return `"$HOME/` + rest + `"`
	case strings.ContainsAny(p, " '\""):
		return `"` + p + `"`
	default:
		return p
	}
}
