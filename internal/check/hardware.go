package check

import (
	"regexp"
	"strconv"
	"strings"
)

// Chip describes the Mac's processor as sysctl reports it.
type Chip struct {
	Brand string
}

var appleChipPattern = regexp.MustCompile(`Apple M(\d+)`)

// Generation is the Apple-Silicon generation (1 for M1 …), or 0 for a
// processor that is not Apple Silicon.
func (c Chip) Generation() int {
	m := appleChipPattern.FindStringSubmatch(c.Brand)
	if m == nil {
		return 0
	}
	n, _ := strconv.Atoi(m[1])
	return n
}

// AppleSilicon reports whether MLX and Splash can run on the chip.
func (c Chip) AppleSilicon() bool {
	return c.Generation() > 0
}

// MinOfficialSplashGeneration is the oldest chip generation Inco AI's
// Splash supports (M3).
const MinOfficialSplashGeneration = 3

// SplashM1Releases is where the unofficial community port splash-m1 for
// M1/M2 is published. It is not from Inco AI.
const SplashM1Releases = "https://github.com/paperniuk/splash/releases"

// OfficialSplash reports whether Inco AI's Splash supports the chip.
func (c Chip) OfficialSplash() bool {
	return c.Generation() >= MinOfficialSplashGeneration
}

// NeedsSplashM1 reports whether the chip needs the community port
// splash-m1 (M1 and M2), since the official Splash starts at M3.
func (c Chip) NeedsSplashM1() bool {
	g := c.Generation()
	return g == 1 || g == 2
}

// Launcher is the Splash binary the chip needs.
func (c Chip) Launcher() string {
	if c.NeedsSplashM1() {
		return "splash-m1"
	}
	return "splash"
}

// Memory is the installed RAM.
type Memory struct {
	Bytes uint64
}

// ParseMemory reads the output of `sysctl -n hw.memsize`.
func ParseMemory(output string) (Memory, error) {
	n, err := strconv.ParseUint(strings.TrimSpace(output), 10, 64)
	if err != nil {
		return Memory{}, err
	}
	return Memory{Bytes: n}, nil
}

// GB is the RAM size in the units Apple advertises (GiB, shown as "GB").
func (m Memory) GB() float64 {
	return float64(m.Bytes) / (1 << 30)
}

// GPUShare is the part of the RAM macOS lets the GPU use by default; the
// rest stays with the system and other apps.
const GPUShare = 0.75

// UsableGB is the memory budget for a model.
func (m Memory) UsableGB() float64 {
	return m.GB() * GPUShare
}
