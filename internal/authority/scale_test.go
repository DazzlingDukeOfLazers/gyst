package authority

import (
	"fmt"
	"os"
	"testing"
	"time"
)

// A hundred thousand files, five thousand of them identical: the shape the
// benchmark tree has. Measurement only unless GYST_BENCH is set.
func TestResolveAtScale(t *testing.T) {
	if os.Getenv("GYST_BENCH") == "" {
		t.Skip("measurement only; set GYST_BENCH=1")
	}
	var files []File
	for i := 0; i < 100000; i++ {
		d := fmt.Sprintf("%064d", i)
		if i%20 == 0 {
			d = "dup"
		}
		files = append(files, File{Key: Key{"s", fmt.Sprintf("p%03d/f%05d", i/500, i)}, Digest: d, ObsID: fmt.Sprintf("obs_%d", i)})
	}
	start := time.Now()
	out := Resolve(Input{Files: files})
	t.Logf("Resolve: %d files in %.2fs", len(out), time.Since(start).Seconds())
}
