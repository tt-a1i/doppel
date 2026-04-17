package signing

import (
	"context"
	"strings"
	"testing"

	"github.com/tt-a1i/appclone/internal/core/macos"
	"github.com/tt-a1i/appclone/internal/core/plistops"
)

func TestDeepResign_OrderAndNoEntitlements(t *testing.T) {
	ex := &macos.FakeExecer{Default: macos.FakeResponse{ExitCode: 0}}
	items := []SignableItem{
		{Path: "/tmp/a/Contents/Frameworks/X.framework", Kind: KindFramework, Depth: 1},
		{Path: "/tmp/a", Kind: KindMainBundle, Depth: 0},
	}
	err := DeepResign(context.Background(), ex, items, ResignOptions{Force: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(ex.Calls) != 2 {
		t.Fatalf("expected 2 calls, got %d", len(ex.Calls))
	}
	// First call: framework, second: main bundle. Neither has --entitlements.
	for i, call := range ex.Calls {
		joined := strings.Join(call.Args, " ")
		if strings.Contains(joined, "--entitlements") {
			t.Errorf("call %d unexpectedly has --entitlements: %v", i, call.Args)
		}
	}
	if !strings.Contains(ex.Calls[0].Args[len(ex.Calls[0].Args)-1], "X.framework") {
		t.Errorf("first signed = %v, want framework", ex.Calls[0].Args)
	}
	if ex.Calls[1].Args[len(ex.Calls[1].Args)-1] != "/tmp/a" {
		t.Errorf("last signed = %v, want main bundle", ex.Calls[1].Args)
	}
}

func TestDeepResign_EntitlementsAppliedOnlyToMain(t *testing.T) {
	ex := &macos.FakeExecer{Default: macos.FakeResponse{ExitCode: 0}}
	items := []SignableItem{
		{Path: "/tmp/a/Contents/Frameworks/X.framework", Kind: KindFramework, Depth: 1},
		{Path: "/tmp/a", Kind: KindMainBundle, Depth: 0},
	}
	err := DeepResign(context.Background(), ex, items, ResignOptions{
		Entitlements: plistops.Plist{"com.apple.security.app-sandbox": true},
		Force:        true,
	})
	if err != nil {
		t.Fatal(err)
	}
	fwArgs := strings.Join(ex.Calls[0].Args, " ")
	if strings.Contains(fwArgs, "--entitlements") {
		t.Errorf("framework should not get entitlements: %v", ex.Calls[0].Args)
	}
	mainArgs := strings.Join(ex.Calls[1].Args, " ")
	if !strings.Contains(mainArgs, "--entitlements") {
		t.Errorf("main bundle missing --entitlements: %v", ex.Calls[1].Args)
	}
}

func TestDeepResign_FailurePropagatesPath(t *testing.T) {
	ex := &macos.FakeExecer{
		Default: macos.FakeResponse{ExitCode: 1, Stderr: []byte("resource fork bad")},
	}
	items := []SignableItem{
		{Path: "/tmp/broken.framework", Kind: KindFramework, Depth: 1},
		{Path: "/tmp/a", Kind: KindMainBundle, Depth: 0},
	}
	err := DeepResign(context.Background(), ex, items, ResignOptions{Force: true})
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "/tmp/broken.framework") {
		t.Errorf("error should mention failing path, got %v", err)
	}
	if len(ex.Calls) != 1 {
		t.Errorf("should stop after first failure, got %d calls", len(ex.Calls))
	}
}
