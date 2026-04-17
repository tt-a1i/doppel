package plistops

import (
	"bytes"
	"fmt"
	"os"

	"howett.net/plist"
)

type Plist map[string]any

// Read loads a plist file and returns the decoded map plus the original
// format (plist.XMLFormat, plist.BinaryFormat, …). Write the same format
// back to preserve the on-disk representation.
func Read(path string) (Plist, int, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, 0, err
	}
	var p Plist
	format, err := plist.Unmarshal(data, &p)
	if err != nil {
		return nil, 0, fmt.Errorf("parse plist %s: %w", path, err)
	}
	return p, format, nil
}

// Write serializes p to path in the given format.
func Write(path string, p Plist, format int) error {
	var buf bytes.Buffer
	enc := plist.NewEncoderForFormat(&buf, format)
	enc.Indent("\t")
	if err := enc.Encode(p); err != nil {
		return fmt.Errorf("encode plist: %w", err)
	}
	if err := os.WriteFile(path, buf.Bytes(), 0o644); err != nil {
		return fmt.Errorf("write plist %s: %w", path, err)
	}
	return nil
}

// SetIdentity updates the clone-relevant identity fields in place.
// Empty strings are treated as "leave alone" for name/displayName,
// but bundleID is always required and always set.
func SetIdentity(p Plist, bundleID, name, displayName string) {
	p["CFBundleIdentifier"] = bundleID
	if name != "" {
		p["CFBundleName"] = name
	}
	if displayName != "" {
		p["CFBundleDisplayName"] = displayName
	}
}
