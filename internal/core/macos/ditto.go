package macos

import (
	"context"
	"fmt"
	"strings"
)

// Copy mirrors src → dst using /usr/bin/ditto, preserving xattrs, ACLs, and
// resource forks that plain cp cannot.
func Copy(ctx context.Context, ex Execer, src, dst string) error {
	_, stderr, code, err := ex.Run(ctx, "ditto", src, dst)
	if err != nil {
		return err
	}
	if code != 0 {
		return fmt.Errorf("ditto failed %s → %s (exit %d): %s", src, dst, code, strings.TrimSpace(string(stderr)))
	}
	return nil
}
