package project

import (
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/DazzlingDukeOfLazers/gyst/internal/manifest"
)

func TestMembershipMatchAtScale(t *testing.T) {
	if os.Getenv("GYST_BENCH") == "" {
		t.Skip("measurement only; set GYST_BENCH=1")
	}
	var patterns []string
	for i := 0; i < 29; i++ {
		patterns = append(patterns, fmt.Sprintf("proj%03d/**", i*7))
	}
	start := time.Now()
	n := 0
	for i := 0; i < 100000; i++ {
		loc := fmt.Sprintf("proj%03d/src/f%04d.txt", i/500, i%500)
		for _, p := range patterns {
			if manifest.Match(p, loc) {
				n++
			}
		}
	}
	t.Logf("Match: 100000 files x 29 patterns, %d matches, in %.2fs", n, time.Since(start).Seconds())
}
