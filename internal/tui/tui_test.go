package tui

import (
	"errors"
	"strings"
	"testing"

	"github.com/tt-a1i/doppel/internal/core/clone"
)

func TestHumanizeErrLaunchTestFailure(t *testing.T) {
	err := clone.StageFailure{
		Stage: clone.StageLaunchTest,
		Err:   errors.New("verify failed: launch test: process exited early"),
	}

	headline, detail := humanizeErr(err)
	if !strings.Contains(headline, "startup test") {
		t.Fatalf("headline = %q, want startup test wording", headline)
	}
	if !strings.Contains(detail, "not reliable for normal use") {
		t.Fatalf("detail = %q, want reliability warning", detail)
	}
}
