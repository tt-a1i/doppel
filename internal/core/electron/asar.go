package electron

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"
)

const (
	asarLeadSize           = 16
	asarIntegrityBlockSize = 4 * 1024 * 1024
	packageJSONPath        = "package.json"
)

type packedFile struct {
	path   string
	node   map[string]any
	offset int
	data   []byte
}

// DerivePackageIdentity returns the internal Electron package identity used
// by app.asar. The visible clone name can stay unchanged, but the internal
// package name and product name must diverge from the source app so Electron
// gets a distinct single-instance lock and userData directory.
func DerivePackageIdentity(cloneName, cloneDisplayName, sourceName, sourceDisplayName string) (packageName, productName string) {
	productBase := firstNonEmpty(strings.TrimSpace(cloneDisplayName), strings.TrimSpace(cloneName), "appclone")
	productName = productBase
	if conflictsWithSource(productName, sourceName, sourceDisplayName) {
		productName = strings.TrimSpace(productName + " Clone")
	}

	packageBase := firstNonEmpty(strings.TrimSpace(cloneName), strings.TrimSpace(cloneDisplayName), productName)
	packageName = sanitizePackageName(packageBase)
	if packageName == "" {
		packageName = "appclone"
	}
	if conflictsWithSource(packageName, sourceName, sourceDisplayName) {
		packageName = sanitizePackageName(packageName + "-clone")
	}
	if packageName == "" {
		packageName = "appclone"
	}
	return packageName, productName
}

// RewritePackageIdentity updates package.json inside an Electron app.asar so
// the clone gets its own internal app identity. Missing app.asar or a root
// package.json is treated as a no-op to keep the clone pipeline best-effort.
func RewritePackageIdentity(asarPath, packageName, productName string) (bool, error) {
	buf, err := os.ReadFile(asarPath)
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, err
	}

	root, dataStart, err := parseArchive(buf)
	if err != nil {
		return false, fmt.Errorf("parse asar: %w", err)
	}
	entry, ok := lookupEntry(root, packageJSONPath)
	if !ok {
		return false, nil
	}
	if _, ok := entry["offset"].(string); !ok {
		return false, nil
	}
	packageBytes, err := entryData(buf, dataStart, entry)
	if err != nil {
		return false, fmt.Errorf("read package.json: %w", err)
	}

	var pkg map[string]any
	if err := json.Unmarshal(packageBytes, &pkg); err != nil {
		return false, fmt.Errorf("parse package.json: %w", err)
	}

	changed := false
	if packageName != "" {
		if got, _ := pkg["name"].(string); got != packageName {
			pkg["name"] = packageName
			changed = true
		}
	}
	if productName != "" {
		if got, _ := pkg["productName"].(string); got != productName {
			pkg["productName"] = productName
			changed = true
		}
	}
	if !changed {
		return false, nil
	}

	newPackageBytes, err := json.MarshalIndent(pkg, "", "  ")
	if err != nil {
		return false, fmt.Errorf("encode package.json: %w", err)
	}
	if len(newPackageBytes) == 0 || newPackageBytes[len(newPackageBytes)-1] != '\n' {
		newPackageBytes = append(newPackageBytes, '\n')
	}

	files, err := collectPackedFiles(buf, dataStart, root)
	if err != nil {
		return false, fmt.Errorf("collect packed asar files: %w", err)
	}
	sort.Slice(files, func(i, j int) bool {
		return files[i].offset < files[j].offset
	})

	payload := make([]byte, 0, len(buf)-dataStart+len(newPackageBytes))
	offset := 0
	for i := range files {
		data := files[i].data
		if files[i].path == packageJSONPath {
			data = newPackageBytes
			files[i].node["size"] = len(data)
			files[i].node["integrity"] = buildIntegrity(data)
		}
		files[i].node["offset"] = strconv.Itoa(offset)
		payload = append(payload, data...)
		offset += len(data)
	}

	if err := writeArchive(asarPath, root, payload); err != nil {
		return false, err
	}
	return true, nil
}

func parseArchive(buf []byte) (map[string]any, int, error) {
	if len(buf) < asarLeadSize {
		return nil, 0, fmt.Errorf("archive too small")
	}
	jsonLen := int(binary.LittleEndian.Uint32(buf[12:16]))
	if jsonLen <= 0 || len(buf) < asarLeadSize+jsonLen {
		return nil, 0, fmt.Errorf("invalid header length %d", jsonLen)
	}
	dataStart := asarLeadSize + align4(jsonLen)
	if dataStart > len(buf) {
		return nil, 0, fmt.Errorf("header exceeds archive size")
	}
	var root map[string]any
	if err := json.Unmarshal(buf[asarLeadSize:asarLeadSize+jsonLen], &root); err != nil {
		return nil, 0, err
	}
	return root, dataStart, nil
}

func lookupEntry(root map[string]any, path string) (map[string]any, bool) {
	cur := root
	for _, part := range strings.Split(path, "/") {
		files, ok := cur["files"].(map[string]any)
		if !ok {
			return nil, false
		}
		next, ok := files[part].(map[string]any)
		if !ok {
			return nil, false
		}
		cur = next
	}
	return cur, true
}

func collectPackedFiles(buf []byte, dataStart int, root map[string]any) ([]packedFile, error) {
	var out []packedFile
	var walk func(prefix string, node map[string]any) error
	walk = func(prefix string, node map[string]any) error {
		if _, isLink := node["link"]; isLink {
			return nil
		}
		if offStr, ok := node["offset"].(string); ok {
			offset, err := strconv.Atoi(offStr)
			if err != nil {
				return fmt.Errorf("parse offset %q: %w", offStr, err)
			}
			data, err := entryData(buf, dataStart, node)
			if err != nil {
				return fmt.Errorf("%s: %w", prefix, err)
			}
			out = append(out, packedFile{path: prefix, node: node, offset: offset, data: data})
			return nil
		}
		files, ok := node["files"].(map[string]any)
		if !ok {
			return nil
		}
		for name, child := range files {
			next, ok := child.(map[string]any)
			if !ok {
				continue
			}
			nextPath := name
			if prefix != "" {
				nextPath = prefix + "/" + name
			}
			if err := walk(nextPath, next); err != nil {
				return err
			}
		}
		return nil
	}
	if err := walk("", root); err != nil {
		return nil, err
	}
	return out, nil
}

func entryData(buf []byte, dataStart int, node map[string]any) ([]byte, error) {
	size, err := intValue(node["size"])
	if err != nil {
		return nil, fmt.Errorf("invalid size: %w", err)
	}
	offStr, ok := node["offset"].(string)
	if !ok {
		return nil, fmt.Errorf("missing offset")
	}
	offset, err := strconv.Atoi(offStr)
	if err != nil {
		return nil, fmt.Errorf("parse offset %q: %w", offStr, err)
	}
	start := dataStart + offset
	end := start + size
	if start < 0 || end < start || end > len(buf) {
		return nil, fmt.Errorf("entry range %d:%d outside archive", start, end)
	}
	return append([]byte(nil), buf[start:end]...), nil
}

func writeArchive(path string, root map[string]any, payload []byte) error {
	headerJSON, err := json.Marshal(root)
	if err != nil {
		return fmt.Errorf("marshal asar header: %w", err)
	}
	aligned := align4(len(headerJSON))
	buf := make([]byte, asarLeadSize+aligned+len(payload))
	binary.LittleEndian.PutUint32(buf[0:4], 4)
	binary.LittleEndian.PutUint32(buf[4:8], uint32(aligned+8))
	binary.LittleEndian.PutUint32(buf[8:12], uint32(aligned+4))
	binary.LittleEndian.PutUint32(buf[12:16], uint32(len(headerJSON)))
	copy(buf[asarLeadSize:], headerJSON)
	copy(buf[asarLeadSize+aligned:], payload)
	return os.WriteFile(path, buf, 0o644)
}

func buildIntegrity(data []byte) map[string]any {
	whole := sha256.Sum256(data)
	blocks := make([]string, 0, max(1, (len(data)+asarIntegrityBlockSize-1)/asarIntegrityBlockSize))
	if len(data) == 0 {
		empty := sha256.Sum256(nil)
		blocks = append(blocks, fmt.Sprintf("%x", empty[:]))
	} else {
		for start := 0; start < len(data); start += asarIntegrityBlockSize {
			end := start + asarIntegrityBlockSize
			if end > len(data) {
				end = len(data)
			}
			block := sha256.Sum256(data[start:end])
			blocks = append(blocks, fmt.Sprintf("%x", block[:]))
		}
	}
	return map[string]any{
		"algorithm": "SHA256",
		"hash":      fmt.Sprintf("%x", whole[:]),
		"blockSize": asarIntegrityBlockSize,
		"blocks":    blocks,
	}
}

func intValue(v any) (int, error) {
	switch n := v.(type) {
	case int:
		return n, nil
	case int64:
		return int(n), nil
	case float64:
		return int(n), nil
	case json.Number:
		i, err := n.Int64()
		return int(i), err
	default:
		return 0, fmt.Errorf("unsupported numeric type %T", v)
	}
}

func align4(n int) int {
	return (n + 3) &^ 3
}

func sanitizePackageName(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	var b strings.Builder
	prevSep := false
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
			prevSep = false
		case r == '.', r == '-', r == '_':
			if b.Len() == 0 || prevSep {
				continue
			}
			b.WriteRune(r)
			prevSep = true
		default:
			if b.Len() == 0 || prevSep {
				continue
			}
			b.WriteByte('-')
			prevSep = true
		}
	}
	return strings.Trim(b.String(), "._-")
}

func conflictsWithSource(candidate, sourceName, sourceDisplayName string) bool {
	return sameNormalized(candidate, sourceName) || sameNormalized(candidate, sourceDisplayName)
}

func sameNormalized(a, b string) bool {
	na := normalizeComparable(a)
	return na != "" && na == normalizeComparable(b)
}

func normalizeComparable(s string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(s) {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
		}
	}
	return b.String()
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}
	return ""
}
