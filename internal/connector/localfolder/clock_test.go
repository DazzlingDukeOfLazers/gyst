package localfolder

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// timeAgo sets a fixture file's mtime well outside the stability window and
// returns a fixed instant to use as the pass clock.
func timeAgo(t *testing.T, dir, name string) time.Time {
	t.Helper()
	old := time.Now().Add(-time.Hour)
	if err := os.Chtimes(filepath.Join(dir, name), old, old); err != nil {
		t.Fatal(err)
	}
	return time.Date(2026, 9, 20, 12, 0, 0, 0, time.UTC)
}
