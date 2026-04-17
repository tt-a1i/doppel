package apperr

import "errors"

var (
	ErrUnsupportedOS = errors.New("appclone requires macOS")
	ErrAppMissing    = errors.New("app path does not exist")
	ErrNotAnApp      = errors.New("not a macOS .app bundle")
	ErrAppUnreadable = errors.New("app path is not readable")
	ErrInvalidInput  = errors.New("invalid input")
	ErrTargetExists  = errors.New("target already exists")
)
