package cli

import (
	"errors"
	"testing"

	"github.com/tt-a1i/doppel/internal/core/clone"
	"github.com/tt-a1i/doppel/internal/core/exitcodes"
)

func TestErrToExitCode_StageErrors(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want int
	}{
		{name: "copy", err: clone.StageFailure{Stage: clone.StageCopy, Err: errors.New("copy failed")}, want: exitcodes.CopyFailed},
		{name: "plist", err: clone.StageFailure{Stage: clone.StagePlist, Err: errors.New("plist failed")}, want: exitcodes.PlistMutationFailed},
		{name: "signing", err: clone.StageFailure{Stage: clone.StageResign, Err: errors.New("sign failed")}, want: exitcodes.SigningFailed},
		{name: "verify", err: clone.StageFailure{Stage: clone.StageVerify, Err: errors.New("verify failed")}, want: exitcodes.VerificationFailed},
		{name: "launch test", err: clone.StageFailure{Stage: clone.StageLaunchTest, Err: errors.New("launch failed")}, want: exitcodes.LaunchTestFailed},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := errToExitCode(tc.err); got != tc.want {
				t.Fatalf("exit code = %d, want %d", got, tc.want)
			}
		})
	}
}
