package macos

import (
	"bytes"
	"context"
	"errors"
	"os/exec"
	"strings"
	"sync"
)

// Execer runs external commands. Exit-code != 0 is reported via exitCode,
// not err. err is set only for process-start failures (binary not found,
// context cancelled, etc.).
type Execer interface {
	Run(ctx context.Context, name string, args ...string) (stdout, stderr []byte, exitCode int, err error)
}

type realExecer struct{}

func NewExecer() Execer { return realExecer{} }

func (realExecer) Run(ctx context.Context, name string, args ...string) ([]byte, []byte, int, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	var so, se bytes.Buffer
	cmd.Stdout = &so
	cmd.Stderr = &se
	err := cmd.Run()
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			return so.Bytes(), se.Bytes(), exitErr.ExitCode(), nil
		}
		return so.Bytes(), se.Bytes(), -1, err
	}
	return so.Bytes(), se.Bytes(), 0, nil
}

// FakeExecer is a test helper. It records every call and returns a scripted
// response for each command key ("name arg1 arg2 ..."). If no script matches,
// Default is returned.
type FakeExecer struct {
	mu        sync.Mutex
	Responses map[string]FakeResponse
	Default   FakeResponse
	Calls     []FakeCall
}

type FakeResponse struct {
	Stdout   []byte
	Stderr   []byte
	ExitCode int
	Err      error
}

type FakeCall struct {
	Name string
	Args []string
}

func (f *FakeExecer) Run(_ context.Context, name string, args ...string) ([]byte, []byte, int, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.Calls = append(f.Calls, FakeCall{Name: name, Args: append([]string(nil), args...)})
	key := name + " " + strings.Join(args, " ")
	if r, ok := f.Responses[key]; ok {
		return r.Stdout, r.Stderr, r.ExitCode, r.Err
	}
	return f.Default.Stdout, f.Default.Stderr, f.Default.ExitCode, f.Default.Err
}
