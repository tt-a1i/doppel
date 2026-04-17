package macos

import (
	"context"
	"reflect"
	"testing"
)

func TestVerify_Success(t *testing.T) {
	ex := &FakeExecer{Default: FakeResponse{ExitCode: 0, Stderr: []byte("valid on disk")}}
	r, err := Verify(context.Background(), ex, "/tmp/foo.app", true, true)
	if err != nil {
		t.Fatal(err)
	}
	if !r.OK || !r.Deep || !r.Strict {
		t.Errorf("unexpected result: %+v", r)
	}
	wantArgs := []string{"--verify", "--verbose=2", "--deep", "--strict", "/tmp/foo.app"}
	if !reflect.DeepEqual(ex.Calls[0].Args, wantArgs) {
		t.Errorf("args = %v, want %v", ex.Calls[0].Args, wantArgs)
	}
}

func TestVerify_Failure(t *testing.T) {
	ex := &FakeExecer{Default: FakeResponse{ExitCode: 1, Stderr: []byte("invalid sig")}}
	r, err := Verify(context.Background(), ex, "/tmp/foo.app", false, false)
	if err != nil {
		t.Fatalf("verify returned err, expected result; got %v", err)
	}
	if r.OK {
		t.Error("expected OK=false on non-zero exit")
	}
}

func TestSign_Success(t *testing.T) {
	ex := &FakeExecer{Default: FakeResponse{ExitCode: 0}}
	err := Sign(context.Background(), ex, "/tmp/foo.app", SignOptions{
		Identity: "-", Force: true, TimestampNone: true, PreserveFlags: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	wantArgs := []string{"--force", "--sign", "-", "--timestamp=none", "--preserve-metadata=flags", "/tmp/foo.app"}
	if !reflect.DeepEqual(ex.Calls[0].Args, wantArgs) {
		t.Errorf("args = %v, want %v", ex.Calls[0].Args, wantArgs)
	}
}

func TestSign_WithEntitlements(t *testing.T) {
	ex := &FakeExecer{Default: FakeResponse{ExitCode: 0}}
	err := Sign(context.Background(), ex, "/tmp/foo.app", SignOptions{
		Identity: "-", Force: true, EntitlementsFile: "/tmp/ent.plist",
	})
	if err != nil {
		t.Fatal(err)
	}
	wantArgs := []string{"--force", "--sign", "-", "--entitlements", "/tmp/ent.plist", "/tmp/foo.app"}
	if !reflect.DeepEqual(ex.Calls[0].Args, wantArgs) {
		t.Errorf("args = %v, want %v", ex.Calls[0].Args, wantArgs)
	}
}

func TestSign_Failure(t *testing.T) {
	ex := &FakeExecer{Default: FakeResponse{ExitCode: 1, Stderr: []byte("resource fork etc")}}
	err := Sign(context.Background(), ex, "/tmp/foo.app", SignOptions{Identity: "-"})
	if err == nil {
		t.Fatal("expected error on signing failure")
	}
}

func TestExtractEntitlements_Unsigned(t *testing.T) {
	ex := &FakeExecer{Default: FakeResponse{ExitCode: 1, Stderr: []byte("code object is not signed at all")}}
	ent, err := ExtractEntitlements(context.Background(), ex, "/tmp/foo.app")
	if err != nil {
		t.Fatalf("unsigned app should not error, got %v", err)
	}
	if ent != nil {
		t.Errorf("expected nil entitlements, got %d bytes", len(ent))
	}
}

func TestExtractEntitlements_Present(t *testing.T) {
	body := []byte(`<?xml version="1.0"?><plist><dict/></plist>`)
	ex := &FakeExecer{Default: FakeResponse{ExitCode: 0, Stdout: body}}
	ent, err := ExtractEntitlements(context.Background(), ex, "/tmp/foo.app")
	if err != nil {
		t.Fatal(err)
	}
	if string(ent) != string(body) {
		t.Errorf("entitlement bytes mismatch")
	}
}

func TestAssess_Accept(t *testing.T) {
	ex := &FakeExecer{Default: FakeResponse{ExitCode: 0, Stderr: []byte("accepted")}}
	r, err := Assess(context.Background(), ex, "/tmp/foo.app")
	if err != nil || !r.Accepted {
		t.Errorf("expected accepted, got %+v err=%v", r, err)
	}
}

func TestAssess_Reject(t *testing.T) {
	ex := &FakeExecer{Default: FakeResponse{ExitCode: 3, Stderr: []byte("rejected")}}
	r, err := Assess(context.Background(), ex, "/tmp/foo.app")
	if err != nil {
		t.Fatalf("rejection should not be an error, got %v", err)
	}
	if r.Accepted {
		t.Error("expected Accepted=false")
	}
}

func TestCopy_Failure(t *testing.T) {
	ex := &FakeExecer{Default: FakeResponse{ExitCode: 1, Stderr: []byte("permission denied")}}
	err := Copy(context.Background(), ex, "/src", "/dst")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestCopy_Success(t *testing.T) {
	ex := &FakeExecer{Default: FakeResponse{ExitCode: 0}}
	if err := Copy(context.Background(), ex, "/src", "/dst"); err != nil {
		t.Fatal(err)
	}
	wantArgs := []string{"/src", "/dst"}
	if !reflect.DeepEqual(ex.Calls[0].Args, wantArgs) {
		t.Errorf("args = %v, want %v", ex.Calls[0].Args, wantArgs)
	}
}
