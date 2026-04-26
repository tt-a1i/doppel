package appinfo

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/tt-a1i/doppel/internal/core/apperr"
)

func TestRequireMacOS(t *testing.T) {
	err := RequireMacOS()
	if runtime.GOOS == "darwin" {
		if err != nil {
			t.Fatalf("expected nil on darwin, got %v", err)
		}
		return
	}
	if !errors.Is(err, apperr.ErrUnsupportedOS) {
		t.Fatalf("expected ErrUnsupportedOS, got %v", err)
	}
}

func writeValidBundle(t *testing.T, dir, name string) string {
	t.Helper()
	app := filepath.Join(dir, name)
	if err := os.MkdirAll(filepath.Join(app, "Contents"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(app, "Contents", "Info.plist"), []byte("<?xml version=\"1.0\"?>\n<plist/>\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	return app
}

func TestValidateAppPath(t *testing.T) {
	tests := []struct {
		name    string
		setup   func(t *testing.T) string
		wantErr error
	}{
		{
			name: "missing path",
			setup: func(t *testing.T) string {
				return filepath.Join(t.TempDir(), "missing.app")
			},
			wantErr: apperr.ErrAppMissing,
		},
		{
			name: "file not directory",
			setup: func(t *testing.T) string {
				p := filepath.Join(t.TempDir(), "file.app")
				if err := os.WriteFile(p, []byte("x"), 0o644); err != nil {
					t.Fatal(err)
				}
				return p
			},
			wantErr: apperr.ErrNotAnApp,
		},
		{
			name: "directory without .app suffix",
			setup: func(t *testing.T) string {
				return writeValidBundle(t, t.TempDir(), "foo")
			},
			wantErr: apperr.ErrNotAnApp,
		},
		{
			name: "directory missing Contents/Info.plist",
			setup: func(t *testing.T) string {
				p := filepath.Join(t.TempDir(), "bad.app")
				if err := os.MkdirAll(p, 0o755); err != nil {
					t.Fatal(err)
				}
				return p
			},
			wantErr: apperr.ErrNotAnApp,
		},
		{
			name: "valid .app bundle",
			setup: func(t *testing.T) string {
				return writeValidBundle(t, t.TempDir(), "good.app")
			},
			wantErr: nil,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			p := tc.setup(t)
			err := ValidateAppPath(p)
			if tc.wantErr == nil {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				return
			}
			if !errors.Is(err, tc.wantErr) {
				t.Fatalf("expected %v, got %v", tc.wantErr, err)
			}
		})
	}
}

func TestValidateBundleID(t *testing.T) {
	tests := []struct {
		id     string
		wantOK bool
	}{
		{id: "com.example.app", wantOK: true},
		{id: "com.example.my-app2", wantOK: true},
		{id: "", wantOK: false},
		{id: "com..example", wantOK: false},
		{id: ".com.example", wantOK: false},
		{id: "com.example.", wantOK: false},
		{id: "com.-example.app", wantOK: false},
		{id: "com.example_.app", wantOK: false},
		{id: "com.example.中文", wantOK: false},
	}

	for _, tc := range tests {
		t.Run(tc.id, func(t *testing.T) {
			err := ValidateBundleID(tc.id)
			if tc.wantOK && err != nil {
				t.Fatalf("expected valid bundle ID, got %v", err)
			}
			if !tc.wantOK && !errors.Is(err, apperr.ErrInvalidInput) {
				t.Fatalf("expected invalid input, got %v", err)
			}
		})
	}
}
