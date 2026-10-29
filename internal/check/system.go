package check

import (
	"context"
	"strings"
)

// OMLXAppPlist is where the oMLX app keeps its version.
const OMLXAppPlist = "/Applications/oMLX.app/Contents/Info.plist"

// System is the hardware and software profile the app checks and stores
// with each result. Fields stay empty when they cannot be read.
type System struct {
	Chip         string  `json:"chip"`
	AppleSilicon bool    `json:"apple_silicon"`
	MemoryGB     float64 `json:"memory_gb"`
	MacOS        string  `json:"macos"`
	Launcher     string  `json:"launcher"` // splash or splash-m1, as the chip needs
	Splash       string  `json:"splash"`   // version of the launcher; empty when not installed
	OMLX         string  `json:"omlx"`     // version of the oMLX app; empty when unknown
}

// UsableGB is the memory budget for a model (see Memory.UsableGB).
func (s System) UsableGB() float64 {
	return s.MemoryGB * GPUShare
}

// ReadSystem reads the profile with the same commands the diagnosis uses.
func ReadSystem(ctx context.Context, commands CommandRunner) System {
	read := func(name string, args ...string) string {
		out, err := commands.Output(ctx, name, args...)
		if err != nil {
			return ""
		}
		return strings.TrimSpace(out)
	}
	brand := read("sysctl", "-n", "machdep.cpu.brand_string")
	chip := Chip{Brand: brand}
	s := System{
		Chip:         strings.TrimPrefix(brand, "Apple "),
		AppleSilicon: chip.AppleSilicon(),
		MacOS:        read("sw_vers", "-productVersion"),
		Launcher:     chip.Launcher(),
		OMLX:         read("plutil", "-extract", "CFBundleShortVersionString", "raw", OMLXAppPlist),
	}
	if mem, err := ParseMemory(read("sysctl", "-n", "hw.memsize")); err == nil {
		s.MemoryGB = mem.GB()
	}
	if v, err := ParseVersion(read(s.Launcher, "--version")); err == nil {
		s.Splash = v.String()
	}
	return s
}
