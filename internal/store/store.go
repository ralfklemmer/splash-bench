// Package store keeps comparison results as JSON files in one directory.
package store

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/ralfklemmer/splash-bench/internal/bench"
	"github.com/ralfklemmer/splash-bench/internal/compare"
)

// fileTime names files so that lexical order is chronological.
const fileTime = "2006-01-02T15-04-05"

// Dir stores one file per result.
type Dir struct {
	path string
}

func NewDir(path string) *Dir {
	return &Dir{path: path}
}

// Save writes r atomically, named after its measurement time.
func (d *Dir) Save(r compare.Result) error {
	if err := os.MkdirAll(d.path, 0o755); err != nil {
		return fmt.Errorf("create %s: %w", d.path, err)
	}
	data, err := json.MarshalIndent(r, "", "  ")
	if err != nil {
		return fmt.Errorf("encode result: %w", err)
	}
	name := filepath.Join(d.path, r.MeasuredAt.Local().Format(fileTime)+".json")
	tmp, err := os.CreateTemp(d.path, ".result-*")
	if err != nil {
		return fmt.Errorf("create temp file in %s: %w", d.path, err)
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return fmt.Errorf("write %s: %w", tmp.Name(), err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close %s: %w", tmp.Name(), err)
	}
	if err := os.Rename(tmp.Name(), name); err != nil {
		return fmt.Errorf("save %s: %w", name, err)
	}
	return nil
}

// Latest reads the newest result; ok is false when there is none yet.
func (d *Dir) Latest() (r compare.Result, ok bool, err error) {
	entries, err := os.ReadDir(d.path)
	if errors.Is(err, fs.ErrNotExist) {
		return compare.Result{}, false, nil
	}
	if err != nil {
		return compare.Result{}, false, fmt.Errorf("read %s: %w", d.path, err)
	}
	var names []string
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".json") && !strings.HasPrefix(e.Name(), ".") {
			names = append(names, e.Name())
		}
	}
	if len(names) == 0 {
		return compare.Result{}, false, nil
	}
	r, err = ReadFile(filepath.Join(d.path, slices.Max(names)))
	return r, err == nil, err
}

// ReadFile reads a result file. It also accepts the plain sample list that
// splashbench writes (results.json); its time is the file's modification time.
func ReadFile(path string) (compare.Result, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return compare.Result{}, fmt.Errorf("read %s: %w", path, err)
	}
	if bytes.HasPrefix(bytes.TrimSpace(data), []byte("[")) {
		return readSamples(path, data)
	}
	var r compare.Result
	if err := json.Unmarshal(data, &r); err != nil {
		return compare.Result{}, fmt.Errorf("decode %s: %w", path, err)
	}
	return r, nil
}

func readSamples(path string, data []byte) (compare.Result, error) {
	var samples []bench.Sample
	if err := json.Unmarshal(data, &samples); err != nil {
		return compare.Result{}, fmt.Errorf("decode %s: %w", path, err)
	}
	info, err := os.Stat(path)
	if err != nil {
		return compare.Result{}, fmt.Errorf("stat %s: %w", path, err)
	}
	return compare.Result{MeasuredAt: info.ModTime(), Samples: samples}, nil
}
