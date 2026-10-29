// Package appbundle assembles "Splash Bench.app": a macOS bundle around the
// splashui binary with an Info.plist and an icon drawn in code.
package appbundle

import (
	"bytes"
	"encoding/xml"
	"fmt"
)

// Meta describes the bundle.
type Meta struct {
	Name       string // shown in Finder, e.g. "Splash Bench"
	Identifier string // reverse-DNS bundle id
	Version    string
	Executable string // file name in Contents/MacOS
}

// DefaultMeta is the Splash Bench bundle.
var DefaultMeta = Meta{
	Name:       "Splash Bench",
	Identifier: "de.klemmer-it.splashbench",
	Version:    "dev",
	Executable: "splashui",
}

// InfoPlist renders Contents/Info.plist. The app is an agent (LSUIElement):
// it has no windows of its own, the browser shows the page, so there is no
// Dock icon that macOS would report as not responding.
func InfoPlist(m Meta) []byte {
	entries := []struct {
		key   string
		value any
	}{
		{"CFBundleDevelopmentRegion", "de"},
		{"CFBundleDisplayName", m.Name},
		{"CFBundleExecutable", m.Executable},
		{"CFBundleIconFile", "AppIcon"},
		{"CFBundleIdentifier", m.Identifier},
		{"CFBundleInfoDictionaryVersion", "6.0"},
		{"CFBundleName", m.Name},
		{"CFBundlePackageType", "APPL"},
		{"CFBundleShortVersionString", m.Version},
		{"CFBundleVersion", m.Version},
		{"LSApplicationCategoryType", "public.app-category.developer-tools"},
		{"LSUIElement", true},
		{"NSHighResolutionCapable", true},
	}
	var b bytes.Buffer
	b.WriteString(xml.Header)
	b.WriteString(`<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">` + "\n")
	b.WriteString(`<plist version="1.0">` + "\n<dict>\n")
	for _, e := range entries {
		fmt.Fprintf(&b, "\t<key>%s</key>\n", escape(e.key))
		switch v := e.value.(type) {
		case bool:
			if v {
				b.WriteString("\t<true/>\n")
			} else {
				b.WriteString("\t<false/>\n")
			}
		case string:
			fmt.Fprintf(&b, "\t<string>%s</string>\n", escape(v))
		}
	}
	b.WriteString("</dict>\n</plist>\n")
	return b.Bytes()
}

func escape(s string) string {
	var b bytes.Buffer
	_ = xml.EscapeText(&b, []byte(s))
	return b.String()
}
