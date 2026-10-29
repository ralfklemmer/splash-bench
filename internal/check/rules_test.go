package check

import (
	"math"
	"reflect"
	"testing"
)

func TestParseVersion(t *testing.T) {
	tests := []struct {
		output  string
		want    Version
		wantErr bool
	}{
		{"Splash 1.3.2", Version{1, 3, 2}, false},
		{"splash-m1 1.3.0\n", Version{1, 3, 0}, false},
		{"Splash 10.12.7 (build abc)", Version{10, 12, 7}, false},
		{"Splash dev", Version{}, true},
		{"", Version{}, true},
	}
	for _, tt := range tests {
		got, err := ParseVersion(tt.output)
		if (err != nil) != tt.wantErr || got != tt.want {
			t.Errorf("ParseVersion(%q) = %v, %v", tt.output, got, err)
		}
	}
}

func TestVersionLess(t *testing.T) {
	tests := []struct {
		a, b Version
		want bool
	}{
		{Version{1, 2, 9}, Version{1, 3, 0}, true},
		{Version{1, 3, 0}, Version{1, 3, 0}, false},
		{Version{1, 3, 1}, Version{1, 3, 0}, false},
		{Version{0, 9, 9}, Version{1, 0, 0}, true},
		{Version{2, 0, 0}, Version{1, 9, 9}, false},
	}
	for _, tt := range tests {
		if got := tt.a.Less(tt.b); got != tt.want {
			t.Errorf("%v.Less(%v) = %v", tt.a, tt.b, got)
		}
	}
}

func TestVersionFinding(t *testing.T) {
	tests := []struct {
		version     Version
		wantStatus  Status
		wantMissing int
	}{
		{Version{1, 2, 0}, StatusFail, 3},
		{Version{1, 3, 0}, StatusWarn, 3},
		{Version{1, 3, 1}, StatusWarn, 2},
		{Version{1, 3, 2}, StatusWarn, 1}, // no Qwen3.8-Flash-Next yet
		{Version{1, 4, 0}, StatusOK, 0},
		{Version{1, 5, 0}, StatusOK, 0},
	}
	for _, tt := range tests {
		if got := versionFinding("splash", tt.version).Status; got != tt.wantStatus {
			t.Errorf("versionFinding(%v) = %v, want %v", tt.version, got, tt.wantStatus)
		}
		if got := len(tt.version.MissingFeatures()); got != tt.wantMissing {
			t.Errorf("%v: %d missing features, want %d", tt.version, got, tt.wantMissing)
		}
	}
}

func TestChip(t *testing.T) {
	tests := []struct {
		brand        string
		generation   int
		needsM1      bool
		appleSilicon bool
		official     bool
	}{
		{"Apple M1", 1, true, true, false},
		{"Apple M2 Pro", 2, true, true, false},
		{"Apple M3 Max", 3, false, true, true},
		{"Apple M5 Max", 5, false, true, true},
		{"Intel(R) Core(TM) i9-9980HK CPU @ 2.40GHz", 0, false, false, false},
	}
	for _, tt := range tests {
		c := Chip{Brand: tt.brand}
		if c.Generation() != tt.generation || c.NeedsSplashM1() != tt.needsM1 || c.AppleSilicon() != tt.appleSilicon || c.OfficialSplash() != tt.official {
			t.Errorf("%q: generation %d, needsM1 %v, appleSilicon %v, official %v",
				tt.brand, c.Generation(), c.NeedsSplashM1(), c.AppleSilicon(), c.OfficialSplash())
		}
	}
}

func TestParseMacOSVersion(t *testing.T) {
	tests := []struct {
		in   string
		want Version
		ok   bool
	}{
		{"26.4", Version{26, 4, 0}, true},
		{"27.0.1\n", Version{27, 0, 1}, true},
		{"15.7.2", Version{15, 7, 2}, true},
		{"Sequoia", Version{}, false},
	}
	for _, tt := range tests {
		got, err := ParseMacOSVersion(tt.in)
		if (err == nil) != tt.ok || got != tt.want {
			t.Errorf("ParseMacOSVersion(%q) = %v, %v", tt.in, got, err)
		}
	}
}

func TestMacOSRule(t *testing.T) {
	m5 := Chip{Brand: "Apple M5 Max"}
	m2 := Chip{Brand: "Apple M2"}
	tests := []struct {
		version string
		chip    Chip
		want    Status
	}{
		{"27.0.1", m5, StatusOK},
		{"26.4", m5, StatusOK},
		{"26.3.1", m5, StatusFail},
		{"15.7", m5, StatusFail},
		{"15.7", m2, StatusWarn}, // splash-m1: requirement unknown
		{"unbekannt", m5, StatusWarn},
	}
	for _, tt := range tests {
		if got := macOSRule(tt.version, tt.chip).Status; got != tt.want {
			t.Errorf("macOSRule(%q, %s) = %v, want %v", tt.version, tt.chip.Brand, got, tt.want)
		}
	}
}

func TestParseMemory(t *testing.T) {
	mem, err := ParseMemory("137438953472\n")
	if err != nil || mem.GB() != 128 || mem.UsableGB() != 96 {
		t.Errorf("ParseMemory = %v (%v GB), %v", mem, mem.GB(), err)
	}
	if _, err := ParseMemory("viel"); err == nil {
		t.Error("expected error for garbage")
	}
}

// The brief states: 27B ≈ 22/25/29 GB, 35B-A3B ≈ 25/26/27 GB (32K/128K/256K).
func TestMemoryEstimateMatchesBrief(t *testing.T) {
	want := map[string][]float64{
		"Qwen3.8-27B":     {22, 25, 29},
		"Qwen3.6-35B-A3B": {25, 26, 27},
	}
	for _, m := range Catalog {
		for tier, gb := range want[m.Family] {
			if got := math.Round(m.MemoryGB(tier)); got != gb {
				t.Errorf("%s %s: %.2f GB, want ≈ %.0f", m.Family, ContextTiers[tier].Label, m.MemoryGB(tier), gb)
			}
		}
	}
}

func TestRecommend(t *testing.T) {
	const gib = 1 << 30
	tests := []struct {
		ramGB int
		want  []int // tier per catalog model (27B, 35B-A3B, Flash-Next, Ornith, Flint), -1 = does not fit
	}{
		{16, []int{-1, -1, -1, -1, -1}},
		{24, []int{-1, -1, -1, -1, -1}},
		{32, []int{0, -1, -1, -1, -1}},
		{36, []int{1, 1, -1, -1, -1}},
		{48, []int{2, 2, -1, -1, 2}},
		{96, []int{2, 2, 2, 2, 2}},
		{128, []int{2, 2, 2, 2, 2}},
	}
	for _, tt := range tests {
		recs := Recommend(Memory{Bytes: uint64(tt.ramGB) * gib}, OMLXCatalog())
		var got []int
		for _, r := range recs {
			got = append(got, r.Tier)
		}
		if !reflect.DeepEqual(got, tt.want) {
			t.Errorf("%d GB: tiers %v, want %v", tt.ramGB, got, tt.want)
		}
	}
}

func TestRepoName(t *testing.T) {
	if got := Repo("Jundot/Qwen3.8-27B-oQ4e-mtp").Name(); got != "Qwen3.8-27B-oQ4e-mtp" {
		t.Errorf("Name = %q", got)
	}
	if got := Repo("flat").Name(); got != "flat" {
		t.Errorf("Name = %q", got)
	}
}

func TestShellPath(t *testing.T) {
	tests := []struct{ path, want string }{
		{"/Users/ralf/.omlx/models/Jundot/X", "~/.omlx/models/Jundot/X"},
		{"/Users/ralf/My Models/X", `"$HOME/My Models/X"`},
		{"/Volumes/SSD/models", "/Volumes/SSD/models"},
		{"/Volumes/Big SSD/models", `"/Volumes/Big SSD/models"`},
	}
	for _, tt := range tests {
		if got := ShellPath(tt.path, "/Users/ralf"); got != tt.want {
			t.Errorf("ShellPath(%q) = %s, want %s", tt.path, got, tt.want)
		}
	}
}

func TestCatalog(t *testing.T) {
	var inOMLX []string
	for _, m := range OMLXCatalog() {
		inOMLX = append(inOMLX, m.Family)
	}
	if want := []string{"Qwen3.8-27B", "Qwen3.6-35B-A3B", "Qwen3.8-Flash-Next", "Ornith-1.5-122B", "Flint"}; !reflect.DeepEqual(inOMLX, want) {
		t.Errorf("OMLXCatalog = %v, want %v", inOMLX, want)
	}
	for _, m := range Catalog {
		if m.Advice == "" || m.Draft == "" || m.TargetGB <= 0 {
			t.Errorf("%s: incomplete catalog entry %+v", m.Family, m)
		}
		if m.InOMLX && len(m.KVCacheGB) != len(ContextTiers) {
			t.Errorf("%s: KV cache for %d tiers, want %d", m.Family, len(m.KVCacheGB), len(ContextTiers))
		}
	}
	tests := []struct {
		family string
		want   Version
	}{
		{"Qwen3.8-27B", MinSplashVersion},
		{"Qwen3.8-Flash-Next", FlashNextVersion},
		{"Flint", Version{1, 4, 1}},
	}
	for _, tt := range tests {
		for _, m := range Catalog {
			if m.Family == tt.family && m.RequiredSplash() != tt.want {
				t.Errorf("%s: RequiredSplash = %v, want %v", tt.family, m.RequiredSplash(), tt.want)
			}
		}
	}
}

func TestTargetID(t *testing.T) {
	ids := map[string]string{}
	for _, m := range Catalog {
		ids[m.Family] = m.TargetID()
	}
	want := map[string]string{
		"Qwen3.8-27B":        "Qwen3.8-27B-oQ4e-mtp",
		"Qwen3.8-Flash-Next": "Qwen3.8-Flash-Next-GSQ-RCO-Q2_0", // oMLX names a split GGUF after its prefix
		"Flint":              "flint-UD-Q4_K_XL",
	}
	for family, id := range want {
		if ids[family] != id {
			t.Errorf("%s: TargetID = %q, want %q", family, ids[family], id)
		}
	}
}

func TestHFDownload(t *testing.T) {
	tests := []struct {
		repo  Repo
		files []string
		want  string
	}{
		{"incoai/Flint-DFlash2", nil, "hf download incoai/Flint-DFlash2 --local-dir ~/m/Flint-DFlash2"},
		{"unsloth/flint-GGUF", []string{"flint-UD-Q4_K_XL.gguf"}, "hf download unsloth/flint-GGUF flint-UD-Q4_K_XL.gguf --local-dir ~/m/flint-GGUF"},
	}
	for _, tt := range tests {
		if got := HFDownload(tt.repo, tt.files, "~/m/"+tt.repo.Name()); got != tt.want {
			t.Errorf("HFDownload = %q, want %q", got, tt.want)
		}
	}
}
