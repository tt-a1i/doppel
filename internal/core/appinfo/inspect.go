package appinfo

import (
	"fmt"
	"os"
	"path/filepath"

	"howett.net/plist"
)

func Inspect(appPath string) (*InspectionReport, error) {
	if err := ValidateAppPath(appPath); err != nil {
		return nil, err
	}
	data, err := os.ReadFile(filepath.Join(appPath, "Contents", "Info.plist"))
	if err != nil {
		return nil, fmt.Errorf("read Info.plist: %w", err)
	}
	var raw map[string]any
	if _, err := plist.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("parse Info.plist: %w", err)
	}

	id := AppIdentity{
		AppPath:        appPath,
		BundleID:       stringValue(raw, "CFBundleIdentifier"),
		BundleName:     stringValue(raw, "CFBundleName"),
		DisplayName:    stringValue(raw, "CFBundleDisplayName"),
		ExecutableName: stringValue(raw, "CFBundleExecutable"),
		Version:        stringValue(raw, "CFBundleShortVersionString"),
		Build:          stringValue(raw, "CFBundleVersion"),
	}

	report := &InspectionReport{Identity: id}

	if id.ExecutableName != "" {
		exec := filepath.Join(appPath, "Contents", "MacOS", id.ExecutableName)
		if info, err := os.Stat(exec); err == nil && !info.IsDir() {
			report.Executable = exec
		}
	}

	if info, err := os.Stat(filepath.Join(appPath, "Contents", "_CodeSignature", "CodeResources")); err == nil && !info.IsDir() {
		report.HasSignature = true
	}

	return report, nil
}

func stringValue(m map[string]any, key string) string {
	if v, ok := m[key]; ok {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}
