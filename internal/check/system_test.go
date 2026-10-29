package check

import (
	"context"
	"testing"
)

func TestReadSystem(t *testing.T) {
	withOMLX := healthyMac()
	withOMLX["plutil -extract CFBundleShortVersionString raw "+OMLXAppPlist] = "0.7.1rc1\n"
	tests := []struct {
		name     string
		commands fakeCommands
		want     System
	}{
		{"healthy Mac", withOMLX, System{Chip: "M5 Max", AppleSilicon: true, MemoryGB: 128, MacOS: "27.0.1", Launcher: "splash", Splash: "1.5.0", OMLX: "0.7.1rc1"}},
		{"M2 asks splash-m1", fakeCommands{
			"sysctl -n machdep.cpu.brand_string": "Apple M2 Pro\n",
			"sysctl -n hw.memsize":               "34359738368\n",
			"splash-m1 --version":                "splash-m1 1.3.1\n",
		}, System{Chip: "M2 Pro", AppleSilicon: true, MemoryGB: 32, Launcher: "splash-m1", Splash: "1.3.1"}},
		{"Intel Mac", fakeCommands{"sysctl -n machdep.cpu.brand_string": "Intel(R) Core(TM) i9\n"},
			System{Chip: "Intel(R) Core(TM) i9", Launcher: "splash"}},
		{"nothing readable", fakeCommands{}, System{Launcher: "splash"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ReadSystem(context.Background(), tt.commands); got != tt.want {
				t.Errorf("ReadSystem =\n%+v\nwant\n%+v", got, tt.want)
			}
		})
	}
}

func TestUsableGB(t *testing.T) {
	if got := (System{MemoryGB: 32}).UsableGB(); got != 24 {
		t.Errorf("UsableGB = %v, want 24", got)
	}
}
