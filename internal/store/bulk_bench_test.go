package store

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/DazzlingDukeOfLazers/gyst/internal/observe"
)

// TestBulkInsertShapes measures how each engine takes twenty thousand
// observations under different statement shapes. Run with -v to see the
// numbers; it asserts nothing but success.
func TestBulkInsertShapes(t *testing.T) {
	if os.Getenv("GYST_BENCH") == "" {
		t.Skip("measurement only; set GYST_BENCH=1")
	}
	var batch []observe.Observation
	for i := 0; i < 20000; i++ {
		batch = append(batch, obs("file", "bulk", fmt.Sprintf("d/%05d.bin", i), "file.content_fingerprint",
			strings.Repeat("c", 64), int64(i%50+1), "fingerprint", nil, clock))
	}
	ctx := context.Background()
	dsns := map[string]func() string{
		EngineSQLite: func() string {
			return "sqlite:" + filepath.Join(t.TempDir(), fmt.Sprintf("b%d.db", time.Now().UnixNano()))
		},
	}
	if pg := os.Getenv("GYST_TEST_DATABASE_URL"); pg != "" {
		dsns[EnginePostgres] = func() string { return pg }
	}
	for _, shape := range []int{1, 20, 100, 500, 750} {
		for name, dsn := range dsns {
			s, err := OpenDSN(ctx, dsn())
			if err != nil {
				t.Fatal(err)
			}
			if name == EnginePostgres {
				if _, err := s.db.exec(ctx, `DELETE FROM current_files; DELETE FROM projector_state`); err != nil {
					t.Fatal(err)
				}
				// observations cannot be deleted; use a fresh source id per shape instead
				for i := range batch {
					batch[i].Source.SourceID = fmt.Sprintf("bulk%d", shape)
					batch[i].Subject.Location.SourceID = batch[i].Source.SourceID
					batch[i].ObservationID = observe.DeriveID(&batch[i], 0)
				}
			}
			s.db.rowsPerStatement = shape
			start := time.Now()
			if _, err := s.Append(ctx, batch); err != nil {
				t.Fatalf("%s shape %d: %v", name, shape, err)
			}
			appended := time.Since(start)
			start = time.Now()
			if _, err := s.ApplyCurrentFiles(ctx); err != nil {
				t.Fatal(err)
			}
			folded := time.Since(start)
			t.Logf("%-9s rows/stmt %4d  append %6.2fs  fold %6.2fs", name, shape, appended.Seconds(), folded.Seconds())
			s.Close()
		}
	}
}
