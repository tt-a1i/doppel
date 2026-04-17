package appinfo

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/tt-a1i/appclone/internal/core/apperr"
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
