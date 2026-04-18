package signing

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"strings"

	"howett.net/plist"

	"github.com/tt-a1i/doppel/internal/core/macos"
	"github.com/tt-a1i/doppel/internal/core/plistops"
)

// strippedEntitlements are entitlements whose value is tied to a specific
// bundle ID, team ID, or keychain group; they become meaningless (and often
// rejected) after an ad-hoc re-sign with a new identity. We drop them
// entirely rather than try to rewrite.
var strippedEntitlements = []string{
	"application-identifier",
	"com.apple.application-identifier",
	"com.apple.developer.team-identifier",
	"com.apple.developer.associated-domains",
	"com.apple.developer.ubiquity-container-identifiers",
	"com.apple.developer.ubiquity-kvstore-identifier",
	"com.apple.application-groups",
	"com.apple.security.application-groups",
	"keychain-access-groups",
}

// FilterEntitlements returns a copy of ent with identity-bound keys removed
// and any string value containing oldID rewritten to reference newID. The
// changes slice describes each modification for reporting.
func FilterEntitlements(ent plistops.Plist, oldID, newID string) (plistops.Plist, []string) {
	if ent == nil {
		return nil, nil
	}
	strip := make(map[string]bool, len(strippedEntitlements))
	for _, k := range strippedEntitlements {
		strip[k] = true
	}
	out := plistops.Plist{}
	var changes []string
	for k, v := range ent {
		if strip[k] {
			changes = append(changes, "stripped:"+k)
			continue
		}
		if s, ok := v.(string); ok && oldID != "" && strings.Contains(s, oldID) {
			out[k] = strings.ReplaceAll(s, oldID, newID)
			changes = append(changes, "rewrote:"+k)
			continue
		}
		out[k] = v
	}
	return out, changes
}

// ExtractAndFilterEntitlements pulls entitlements from sourceApp (if any),
// filters them for the new identity, and returns the resulting plist plus a
// list of changes. Returns (nil, nil, nil) if the source has no entitlements.
func ExtractAndFilterEntitlements(ctx context.Context, ex macos.Execer, sourceApp, oldID, newID string) (plistops.Plist, []string, error) {
	data, err := macos.ExtractEntitlements(ctx, ex, sourceApp)
	if err != nil {
		return nil, nil, err
	}
	if len(data) == 0 {
		return nil, nil, nil
	}
	var ent plistops.Plist
	if _, err := plist.Unmarshal(data, &ent); err != nil {
		return nil, nil, fmt.Errorf("parse source entitlements: %w", err)
	}
	filtered, changes := FilterEntitlements(ent, oldID, newID)
	return filtered, changes, nil
}

// WriteEntitlementsFile encodes ent as XML plist into a temp file suitable
// for passing to codesign --entitlements. Caller is responsible for
// os.Remove-ing the returned path.
func WriteEntitlementsFile(ent plistops.Plist) (string, error) {
	f, err := os.CreateTemp("", "doppel-ent-*.plist")
	if err != nil {
		return "", err
	}
	var buf bytes.Buffer
	enc := plist.NewEncoderForFormat(&buf, plist.XMLFormat)
	enc.Indent("\t")
	if err := enc.Encode(ent); err != nil {
		f.Close()
		os.Remove(f.Name())
		return "", err
	}
	if _, err := f.Write(buf.Bytes()); err != nil {
		f.Close()
		os.Remove(f.Name())
		return "", err
	}
	if err := f.Close(); err != nil {
		os.Remove(f.Name())
		return "", err
	}
	return f.Name(), nil
}
