package check

import (
	"reflect"
	"testing"
	"testing/fstest"
)

func TestReadModelDirs(t *testing.T) {
	tests := []struct {
		name     string
		settings string
		want     []string
		wantErr  bool
	}{
		{"model_dirs", `{"model":{"model_dirs":["/a","~/b"],"model_dir":"/c"}}`, []string{"/a", "/Users/ralf/b"}, false},
		{"fallback model_dir", `{"model":{"model_dir":"/c"}}`, []string{"/c"}, false},
		{"none", `{"model":{}}`, nil, false},
		{"broken", `{`, nil, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fsys := fstest.MapFS{"Users/ralf/.omlx/settings.json": {Data: []byte(tt.settings)}}
			got, err := ReadModelDirs(fsys, home)
			if (err != nil) != tt.wantErr || !reflect.DeepEqual(got, tt.want) {
				t.Errorf("got %v, %v", got, err)
			}
		})
	}
	if _, err := ReadModelDirs(fstest.MapFS{}, home); err == nil {
		t.Error("expected error for missing settings")
	}
}

func TestLocate(t *testing.T) {
	fsys := fstest.MapFS{
		"m/one/Jundot/A/config.json": {},
		"m/two/B/config.json":        {},
		"m/one/incoai/C/weights":     {},
		"m/two/incoai/C/config.json": {},
		"m/one/incoai/D/weights":     {},
	}
	dirs := []string{"/m/one", "/m/two"}
	tests := []struct {
		repo Repo
		want Location
	}{
		{"Jundot/A", Location{"/m/one/Jundot/A", FolderComplete}},
		{"Jundot/B", Location{"/m/two/B", FolderComplete}},
		{"incoai/C", Location{"/m/two/incoai/C", FolderComplete}},
		{"incoai/D", Location{"/m/one/incoai/D", FolderIncomplete}},
		{"incoai/E", Location{State: FolderMissing}},
	}
	for _, tt := range tests {
		if got := Locate(fsys, dirs, tt.repo); got != tt.want {
			t.Errorf("Locate(%s) = %+v, want %+v", tt.repo, got, tt.want)
		}
	}
}

func TestDirSizeGB(t *testing.T) {
	fsys := fstest.MapFS{
		"p/a":     {Data: make([]byte, 1000)},
		"p/sub/b": {Data: make([]byte, 500)},
	}
	got, err := DirSizeGB(fsys, "/p")
	if err != nil || got != 1.5e-6 {
		t.Errorf("DirSizeGB = %v, %v", got, err)
	}
}
