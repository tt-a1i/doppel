package electron

import (
	"encoding/binary"
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

type testAsarFile struct {
	path     string
	content  []byte
	unpacked bool
}

func TestDerivePackageIdentity(t *testing.T) {
	pkgName, productName := DerivePackageIdentity("clone-01", "Cherry Studio", "Cherry Studio", "Cherry Studio")
	if pkgName != "clone-01" {
		t.Fatalf("packageName = %q, want %q", pkgName, "clone-01")
	}
	if productName != "Cherry Studio Clone" {
		t.Fatalf("productName = %q, want %q", productName, "Cherry Studio Clone")
	}

	pkgName, productName = DerivePackageIdentity("Cherry Studio", "Clone Display", "Cherry Studio", "Cherry Studio")
	if pkgName != "cherry-studio-clone" {
		t.Fatalf("conflicting packageName = %q, want %q", pkgName, "cherry-studio-clone")
	}
	if productName != "Clone Display" {
		t.Fatalf("productName = %q, want %q", productName, "Clone Display")
	}
}

func TestRewritePackageIdentity_UpdatesPackedPackageJSON(t *testing.T) {
	dir := t.TempDir()
	asarPath := filepath.Join(dir, "app.asar")
	writeTestAsar(t, asarPath, []testAsarFile{
		{
			path:    "package.json",
			content: []byte("{\n  \"name\": \"CherryStudio\",\n  \"productName\": \"Cherry Studio\"\n}\n"),
		},
		{
			path:    "dir/after.txt",
			content: []byte("after-data"),
		},
	})

	changed, err := RewritePackageIdentity(asarPath, "clone-01", "Cherry Studio Clone")
	if err != nil {
		t.Fatalf("RewritePackageIdentity: %v", err)
	}
	if !changed {
		t.Fatal("expected RewritePackageIdentity to report a change")
	}

	pkg := readTestAsarJSONFile(t, asarPath, "package.json")
	if got := pkg["name"]; got != "clone-01" {
		t.Fatalf("package name = %v, want %q", got, "clone-01")
	}
	if got := pkg["productName"]; got != "Cherry Studio Clone" {
		t.Fatalf("productName = %v, want %q", got, "Cherry Studio Clone")
	}
	if got := string(readTestAsarRawFile(t, asarPath, "dir/after.txt")); got != "after-data" {
		t.Fatalf("after.txt = %q, want %q", got, "after-data")
	}
}

func TestRewritePackageIdentity_NoPackageJSONNoop(t *testing.T) {
	dir := t.TempDir()
	asarPath := filepath.Join(dir, "app.asar")
	writeTestAsar(t, asarPath, []testAsarFile{{path: "dir/after.txt", content: []byte("after-data")}})

	changed, err := RewritePackageIdentity(asarPath, "clone-01", "Cherry Studio Clone")
	if err != nil {
		t.Fatalf("RewritePackageIdentity: %v", err)
	}
	if changed {
		t.Fatal("expected RewritePackageIdentity to report no change")
	}
}

func TestRewritePackageIdentity_UnpackedPackageJSONNoop(t *testing.T) {
	dir := t.TempDir()
	asarPath := filepath.Join(dir, "app.asar")
	writeTestAsar(t, asarPath, []testAsarFile{{
		path:     "package.json",
		content:  []byte("{\n  \"name\": \"CherryStudio\"\n}\n"),
		unpacked: true,
	}})

	changed, err := RewritePackageIdentity(asarPath, "clone-01", "Cherry Studio Clone")
	if err != nil {
		t.Fatalf("RewritePackageIdentity: %v", err)
	}
	if changed {
		t.Fatal("expected RewritePackageIdentity to report no change for unpacked package.json")
	}
}

func writeTestAsar(t *testing.T, asarPath string, files []testAsarFile) {
	t.Helper()

	root := map[string]any{"files": map[string]any{}}
	offset := 0
	payload := make([]byte, 0)
	for _, file := range files {
		dir := root["files"].(map[string]any)
		parts := strings.Split(file.path, "/")
		for i, part := range parts {
			if i == len(parts)-1 {
				node := map[string]any{"size": len(file.content)}
				if !file.unpacked {
					node["offset"] = strconv.Itoa(offset)
					node["integrity"] = buildIntegrity(file.content)
					payload = append(payload, file.content...)
					offset += len(file.content)
				} else {
					node["unpacked"] = true
				}
				dir[part] = node
				continue
			}
			next, ok := dir[part].(map[string]any)
			if !ok {
				next = map[string]any{"files": map[string]any{}}
				dir[part] = next
			}
			dir = next["files"].(map[string]any)
		}
	}

	headerJSON, err := json.Marshal(root)
	if err != nil {
		t.Fatalf("marshal asar header: %v", err)
	}
	aligned := align4(len(headerJSON))
	buf := make([]byte, asarLeadSize+aligned+len(payload))
	binary.LittleEndian.PutUint32(buf[0:4], 4)
	binary.LittleEndian.PutUint32(buf[4:8], uint32(aligned+8))
	binary.LittleEndian.PutUint32(buf[8:12], uint32(aligned+4))
	binary.LittleEndian.PutUint32(buf[12:16], uint32(len(headerJSON)))
	copy(buf[asarLeadSize:], headerJSON)
	copy(buf[asarLeadSize+aligned:], payload)
	if err := os.WriteFile(asarPath, buf, 0o644); err != nil {
		t.Fatalf("write test asar: %v", err)
	}
}

func readTestAsarJSONFile(t *testing.T, asarPath, path string) map[string]any {
	t.Helper()
	data := readTestAsarRawFile(t, asarPath, path)
	var out map[string]any
	if err := json.Unmarshal(data, &out); err != nil {
		t.Fatalf("unmarshal %s: %v", path, err)
	}
	return out
}

func readTestAsarRawFile(t *testing.T, asarPath, path string) []byte {
	t.Helper()
	buf, err := os.ReadFile(asarPath)
	if err != nil {
		t.Fatalf("read asar: %v", err)
	}
	jsonLen := int(binary.LittleEndian.Uint32(buf[12:16]))
	dataStart := asarLeadSize + align4(jsonLen)

	var root map[string]any
	if err := json.Unmarshal(buf[asarLeadSize:asarLeadSize+jsonLen], &root); err != nil {
		t.Fatalf("unmarshal header: %v", err)
	}
	entry, ok := lookupEntry(root, path)
	if !ok {
		t.Fatalf("entry %q not found", path)
	}
	offStr, ok := entry["offset"].(string)
	if !ok {
		t.Fatalf("entry %q has no packed offset", path)
	}
	offset, err := strconv.Atoi(offStr)
	if err != nil {
		t.Fatalf("parse offset %q: %v", offStr, err)
	}
	size, err := intValue(entry["size"])
	if err != nil {
		t.Fatalf("parse size: %v", err)
	}
	return append([]byte(nil), buf[dataStart+offset:dataStart+offset+size]...)
}
