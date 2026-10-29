package check

import "strings"

// Repo is a Hugging Face repository such as "Jundot/Qwen3.8-27B-oQ4e-mtp".
type Repo string

// Name is the folder name oMLX shows as model id.
func (r Repo) Name() string {
	_, name, found := strings.Cut(string(r), "/")
	if !found {
		return string(r)
	}
	return name
}

// ContextTier is a context length the memory estimate is given for.
type ContextTier struct {
	Label  string
	Tokens int
}

// ContextTiers are the context lengths of the brief's memory table.
var ContextTiers = []ContextTier{{"32K", 32 * 1024}, {"128K", 128 * 1024}, {"256K", 256 * 1024}}

// SplashOverheadGB is the fixed memory Splash needs beyond weights and KV.
const SplashOverheadGB = 3.0

// The catalog was last checked on this date against these releases. Newer
// releases may serve more families; oMLX's own verdict per model (see the
// readiness checks) does not depend on this list.
const (
	CatalogAsOf   = "29.10.2026"
	CatalogSplash = "1.5.0"
	CatalogOMLX   = "0.7.3rc1"
)

// TargetModel is a model Splash can accelerate, with the draft it needs.
type TargetModel struct {
	Family   string
	Target   Repo
	TargetGB float64
	// TargetFiles limits the download to these files of the repository,
	// for a GGUF variant; empty downloads the whole repository.
	TargetFiles []string
	// ServedID is the id oMLX lists the target under; empty means the
	// repository name. oMLX names a GGUF after its file.
	ServedID    string
	Draft       Repo
	DraftGB     float64
	FullPackage Repo
	// KVCacheGB is the int8 KV cache per context tier (same order as
	// ContextTiers). The first two families are derived from the measured
	// totals in the brief, the others scaled by their full-attention layers.
	KVCacheGB []float64
	Advice    string
	// Measured is true for the families measured for the video; only their
	// downloads are suggested without asking.
	Measured bool
	// InOMLX is false for families Splash serves only standalone (splash
	// serve, LM Studio) because oMLX does not hand them to Splash yet.
	InOMLX bool
	// MinSplash is the oldest Splash release serving the family in oMLX;
	// zero means MinSplashVersion.
	MinSplash Version
}

// Catalog lists the models Splash serves, as of CatalogAsOf.
var Catalog = []TargetModel{
	{
		Family:      "Qwen3.8-27B",
		Target:      "Jundot/Qwen3.8-27B-oQ4e-mtp",
		TargetGB:    16.6,
		Draft:       "incoai/Qwen3.8-27B-DFlash2",
		DraftGB:     3.8,
		FullPackage: "incoai/Qwen3.8-27B-Splash",
		KVCacheGB:   []float64{1.1, 4.1, 8.1},
		Advice:      "Splash einschalten: 1,33–1,87× mehr tok/s gemessen",
		Measured:    true,
		InOMLX:      true,
	},
	{
		Family:      "Qwen3.6-35B-A3B",
		Target:      "Jundot/Qwen3.6-35B-A3B-oQ4e-mtp",
		TargetGB:    21.1,
		Draft:       "incoai/Qwen3.6-35B-A3B-DFlash2",
		DraftGB:     1.0,
		FullPackage: "incoai/Qwen3.6-35B-A3B-Splash",
		KVCacheGB:   []float64{0.6, 1.6, 2.6},
		Advice:      "oMLX (MTP) ist hier schon schnell, Splash bringt nur wenig: 1,05–1,32× gemessen",
		Measured:    true,
		InOMLX:      true,
	},
	{
		Family:   "Qwen3.8-Flash-Next",
		Target:   "ISTA-DASLab/Qwen3.8-Flash-Next-GSQ-RCO-GGUF",
		TargetGB: 62.6,
		TargetFiles: []string{
			"Q2_0/Qwen3.8-Flash-Next-GSQ-RCO-Q2_0-00001-of-00002.gguf",
			"Q2_0/Qwen3.8-Flash-Next-GSQ-RCO-Q2_0-00002-of-00002.gguf",
			"mmproj-Qwen3.8-Flash-Next-BF16.gguf",
		},
		ServedID:  "Qwen3.8-Flash-Next-GSQ-RCO-Q2_0",
		Draft:     "incoai/Qwen3.8-Flash-Next-DFlash2",
		DraftGB:   1.1,
		KVCacheGB: []float64{0.7, 1.8, 3.3},
		Advice:    "Neu, noch nicht gemessen – selbst messen. Splash lädt Flash-Next nur als GGUF; hier die 2-Bit-Variante, die Inco empfiehlt.",
		InOMLX:    true,
		MinSplash: FlashNextVersion,
	},
	{
		Family:    "Ornith-1.5-122B",
		Target:    "ornith-ai/Ornith-1.5-122B-MLX-4bit",
		TargetGB:  64.9,
		Draft:     "incoai/Ornith-1.5-122B-DFlash2",
		DraftGB:   1.7,
		KVCacheGB: []float64{0.7, 1.8, 3.3},
		Advice:    "Neu, noch nicht gemessen – selbst messen. Feinabstimmung von Qwen3.5-122B-A10B.",
		InOMLX:    true,
		MinSplash: Version{1, 4, 1},
	},
	{
		Family:      "Flint",
		Target:      "unsloth/flint-GGUF",
		TargetGB:    23.3,
		TargetFiles: []string{"flint-UD-Q4_K_XL.gguf"},
		ServedID:    "flint-UD-Q4_K_XL",
		Draft:       "incoai/Flint-DFlash2",
		DraftGB:     0.8,
		KVCacheGB:   []float64{0.8, 2.2, 3.8},
		Advice:      "Neu, noch nicht gemessen – selbst messen. Nur Text, Splash lädt Flint nur als GGUF.",
		InOMLX:      true,
		MinSplash:   Version{1, 4, 1},
	},
}

// TargetID is the id oMLX lists the target under.
func (m TargetModel) TargetID() string {
	if m.ServedID != "" {
		return m.ServedID
	}
	return m.Target.Name()
}

// HFDownload is the `hf download` line that fetches repo (only files, if
// given) into dest, a shell path.
func HFDownload(repo Repo, files []string, dest string) string {
	args := []string{"hf download", string(repo)}
	args = append(args, files...)
	return strings.Join(append(args, "--local-dir", dest), " ")
}

// OMLXCatalog lists the catalog models oMLX can hand to Splash.
func OMLXCatalog() []TargetModel {
	var out []TargetModel
	for _, m := range Catalog {
		if m.InOMLX {
			out = append(out, m)
		}
	}
	return out
}

// RequiredSplash is the oldest Splash release that serves the model.
func (m TargetModel) RequiredSplash() Version {
	if m.MinSplash.IsZero() {
		return MinSplashVersion
	}
	return m.MinSplash
}

// MemoryGB estimates the memory the model needs with Splash:
// weights + draft/3 + 3 GB + KV cache (int8) for the context tier.
func (m TargetModel) MemoryGB(tier int) float64 {
	kv := 0.0
	if tier < len(m.KVCacheGB) {
		kv = m.KVCacheGB[tier]
	}
	return m.TargetGB + m.DraftGB/3 + SplashOverheadGB + kv
}
