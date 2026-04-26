package appinfo

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"unicode"

	"github.com/tt-a1i/doppel/internal/core/apperr"
)

func RequireMacOS() error {
	if runtime.GOOS != "darwin" {
		return fmt.Errorf("%w: running on %s", apperr.ErrUnsupportedOS, runtime.GOOS)
	}
	return nil
}

func ValidateAppPath(path string) error {
	info, err := os.Stat(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("%w: %s", apperr.ErrAppMissing, path)
		}
		return fmt.Errorf("%w: %v", apperr.ErrAppUnreadable, err)
	}
	if !info.IsDir() {
		return fmt.Errorf("%w: %s is not a directory", apperr.ErrNotAnApp, path)
	}
	if !strings.HasSuffix(path, ".app") {
		return fmt.Errorf("%w: %s lacks .app suffix", apperr.ErrNotAnApp, path)
	}
	plist := filepath.Join(path, "Contents", "Info.plist")
	pinfo, err := os.Stat(plist)
	if err != nil || pinfo.IsDir() {
		return fmt.Errorf("%w: missing Contents/Info.plist under %s", apperr.ErrNotAnApp, path)
	}
	return nil
}

func ValidateBundleID(id string) error {
	id = strings.TrimSpace(id)
	if id == "" {
		return fmt.Errorf("%w: bundle ID is required", apperr.ErrInvalidInput)
	}
	for _, part := range strings.Split(id, ".") {
		if part == "" {
			return fmt.Errorf("%w: bundle ID contains an empty component: %s", apperr.ErrInvalidInput, id)
		}
		if strings.HasPrefix(part, "-") || strings.HasSuffix(part, "-") {
			return fmt.Errorf("%w: bundle ID components cannot start or end with '-': %s", apperr.ErrInvalidInput, id)
		}
		for _, r := range part {
			if r > unicode.MaxASCII || !(unicode.IsLetter(r) || unicode.IsDigit(r) || r == '-') {
				return fmt.Errorf("%w: bundle ID contains unsupported character %q: %s", apperr.ErrInvalidInput, r, id)
			}
		}
	}
	return nil
}
