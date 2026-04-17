package clone

import (
	"context"
	"reflect"
	"testing"

	"github.com/tt-a1i/appclone/internal/core/macos"
)

func TestCopyBundle_InvokesDitto(t *testing.T) {
	ex := &macos.FakeExecer{Default: macos.FakeResponse{ExitCode: 0}}
	plan := &ClonePlan{SourceApp: "/tmp/src.app", TargetApp: "/tmp/dst.app"}
	if err := CopyBundle(context.Background(), plan, ex); err != nil {
		t.Fatal(err)
	}
	if len(ex.Calls) != 1 || ex.Calls[0].Name != "ditto" {
		t.Fatalf("expected ditto call, got %+v", ex.Calls)
	}
	wantArgs := []string{"/tmp/src.app", "/tmp/dst.app"}
	if !reflect.DeepEqual(ex.Calls[0].Args, wantArgs) {
		t.Errorf("args = %v, want %v", ex.Calls[0].Args, wantArgs)
	}
}

func TestCopyBundle_DryRunSkipsExec(t *testing.T) {
	ex := &macos.FakeExecer{Default: macos.FakeResponse{ExitCode: 0}}
	plan := &ClonePlan{SourceApp: "/tmp/src.app", TargetApp: "/tmp/dst.app", DryRun: true}
	if err := CopyBundle(context.Background(), plan, ex); err != nil {
		t.Fatal(err)
	}
	if len(ex.Calls) != 0 {
		t.Errorf("expected no calls in dry-run, got %+v", ex.Calls)
	}
}

func TestCopyBundle_PropagatesFailure(t *testing.T) {
	ex := &macos.FakeExecer{Default: macos.FakeResponse{ExitCode: 1, Stderr: []byte("denied")}}
	plan := &ClonePlan{SourceApp: "/tmp/src.app", TargetApp: "/tmp/dst.app"}
	if err := CopyBundle(context.Background(), plan, ex); err == nil {
		t.Fatal("expected error on ditto failure")
	}
}
