package plistops

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"howett.net/plist"
)

const samplePlist = `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
    <key>CFBundleIdentifier</key><string>com.example.app</string>
    <key>CFBundleName</key><string>Example</string>
    <key>CFBundleExecutable</key><string>Example</string>
    <key>CFBundleShortVersionString</key><string>1.0</string>
    <key>LSUIElement</key><true/>
</dict>
</plist>`

func writeTempPlist(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "Info.plist")
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestReadXML(t *testing.T) {
	path := writeTempPlist(t, samplePlist)
	p, format, err := Read(path)
	if err != nil {
		t.Fatal(err)
	}
	if format != plist.XMLFormat {
		t.Errorf("format = %d, want XMLFormat (%d)", format, plist.XMLFormat)
	}
	if p["CFBundleIdentifier"] != "com.example.app" {
		t.Errorf("bundle id = %v", p["CFBundleIdentifier"])
	}
	if p["LSUIElement"] != true {
		t.Errorf("LSUIElement = %v, want true", p["LSUIElement"])
	}
}

func TestRoundtripXML(t *testing.T) {
	path := writeTempPlist(t, samplePlist)
	p, format, err := Read(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := Write(path, p, format); err != nil {
		t.Fatal(err)
	}
	p2, _, err := Read(path)
	if err != nil {
		t.Fatal(err)
	}
	if p2["CFBundleIdentifier"] != p["CFBundleIdentifier"] {
		t.Errorf("roundtrip changed bundle id")
	}
	if p2["LSUIElement"] != p["LSUIElement"] {
		t.Errorf("roundtrip changed LSUIElement")
	}
}

func TestRoundtripBinary(t *testing.T) {
	path := filepath.Join(t.TempDir(), "Info.plist")
	src := Plist{
		"CFBundleIdentifier": "com.example.bin",
		"CFBundleExecutable": "Bin",
		"Count":              42,
		"Nested":             map[string]any{"k": "v"},
	}
	var buf bytes.Buffer
	enc := plist.NewEncoderForFormat(&buf, plist.BinaryFormat)
	if err := enc.Encode(src); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, buf.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}

	got, format, err := Read(path)
	if err != nil {
		t.Fatal(err)
	}
	if format != plist.BinaryFormat {
		t.Errorf("format = %d, want BinaryFormat (%d)", format, plist.BinaryFormat)
	}
	if got["CFBundleIdentifier"] != "com.example.bin" {
		t.Errorf("bundle id lost")
	}

	if err := Write(path, got, format); err != nil {
		t.Fatal(err)
	}
	got2, format2, err := Read(path)
	if err != nil {
		t.Fatal(err)
	}
	if format2 != plist.BinaryFormat {
		t.Errorf("roundtrip changed format to %d", format2)
	}
	if got2["CFBundleIdentifier"] != got["CFBundleIdentifier"] {
		t.Errorf("binary roundtrip changed bundle id")
	}
}

func TestSetIdentity(t *testing.T) {
	p := Plist{
		"CFBundleIdentifier":         "com.old.app",
		"CFBundleName":               "Old",
		"CFBundleExecutable":         "OldBin",
		"CFBundleShortVersionString": "1.0",
		"LSUIElement":                true,
	}
	SetIdentity(p, "com.new.app", "New", "New Display")
	if p["CFBundleIdentifier"] != "com.new.app" {
		t.Errorf("bundle id = %v", p["CFBundleIdentifier"])
	}
	if p["CFBundleName"] != "New" {
		t.Errorf("bundle name = %v", p["CFBundleName"])
	}
	if p["CFBundleDisplayName"] != "New Display" {
		t.Errorf("display name = %v", p["CFBundleDisplayName"])
	}
	if p["CFBundleExecutable"] != "OldBin" {
		t.Errorf("unrelated field CFBundleExecutable was modified")
	}
	if p["LSUIElement"] != true {
		t.Errorf("unrelated bool was modified")
	}
}

func TestSetIdentity_EmptyNamePreservesExisting(t *testing.T) {
	p := Plist{
		"CFBundleIdentifier": "com.old.app",
		"CFBundleName":       "Original",
	}
	SetIdentity(p, "com.new.app", "", "")
	if p["CFBundleName"] != "Original" {
		t.Errorf("empty name should preserve existing, got %v", p["CFBundleName"])
	}
	if _, has := p["CFBundleDisplayName"]; has {
		t.Errorf("empty displayName should not add key; got %v", p["CFBundleDisplayName"])
	}
}
