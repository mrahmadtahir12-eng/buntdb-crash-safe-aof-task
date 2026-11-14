package verifier

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/tidwall/buntdb"
)

const nTxns = 40

// buildDB writes transactions 1..n of g under the given policy and returns the
// path and the file size observed right after each Update returned.
func buildDB(t *testing.T, policy buntdb.SyncPolicy, g Gen, n int) (dir, path string, sizes []int64) {
	t.Helper()
	dir = t.TempDir()
	path = filepath.Join(dir, "data.db")
	db, err := OpenWith(path, policy)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	sizes = make([]int64, n+1)
	for i := 1; i <= n; i++ {
		if err := ApplyTxn(db, g.Txn(i)); err != nil {
			t.Fatalf("Update %d: %v", i, err)
		}
		if sizes[i], err = fileSize(path); err != nil {
			t.Fatal(err)
		}
	}
	if err := db.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	return dir, path, sizes
}

// V1: write, close, reopen (same process and a fresh process), compare; a
// rolled-back transaction and a failed Update leave no trace.
func TestV1_RoundTripAndRollback(t *testing.T) {
	seed := SeedFromEnv()
	t.Logf("seed %d", seed)
	g := DefaultGen(seed)
	_, path, _ := buildDB(t, buntdb.Always, g, nTxns)
	want := g.StateAfter(nTxns)

	db, err := buntdb.Open(path)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	got, err := ReadState(db)
	if err != nil {
		t.Fatal(err)
	}
	if !SameState(want, got) {
		t.Fatalf("reopened state differs: %s", DiffState(want, got))
	}

	// explicit rollback
	tx, err := db.Begin(true)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := tx.Set("rolled", "back", nil); err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Delete("k01"); err != nil && err != buntdb.ErrNotFound {
		t.Fatal(err)
	}
	if err := tx.Rollback(); err != nil {
		t.Fatal(err)
	}
	// an Update whose function fails is rolled back
	boom := errors.New("boom")
	if err := db.Update(func(tx *buntdb.Tx) error {
		if _, _, err := tx.Set("failed", "update", nil); err != nil {
			return err
		}
		return boom
	}); err != boom {
		t.Fatalf("Update should return the function's error, got %v", err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	fresh := t.TempDir()
	p := filepath.Join(fresh, "data.db")
	if err := copyFile(path, p); err != nil {
		t.Fatal(err)
	}
	res, err := runChild("open", p, seed)
	if err != nil {
		t.Fatal(err)
	}
	if res.Err != "" {
		t.Fatalf("fresh process could not open: %s", res.Err)
	}
	if got := decState(res.State); !SameState(want, got) {
		t.Fatalf("fresh-process state differs: %s", DiffState(want, got))
	}
}

// V10: one file at the path after a clean close, and commit cost that does not
// grow with the database size.
func TestV10_OneFileAndCommitCost(t *testing.T) {
	seed := SeedFromEnv()
	g := DefaultGen(seed)
	dir, _, _ := buildDB(t, buntdb.EverySecond, g, nTxns)
	names, err := listDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(names) != 1 || names[0] != "data.db" {
		t.Fatalf("after a clean close the directory should hold exactly data.db, got %v", names)
	}

	// Commit cost. SyncPolicy Never so that fsync does not dominate.
	small := timeCommits(t, 0)
	big := timeCommits(t, envInt("BUNTDB_VERIFY_BIGDB_MB", 50))
	ratio := float64(big) / float64(small)
	t.Logf("500 small commits: tiny db %v, big db %v, ratio %.1f", small, big, ratio)
	if ratio > 20 {
		t.Fatalf("commit cost grows with database size: ratio %.1f (bound 20)", ratio)
	}
}

func timeCommits(t *testing.T, preloadMB int) time.Duration {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "data.db")
	db, err := OpenWith(path, buntdb.Never)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if preloadMB > 0 {
		val := strings.Repeat("v", 100*1024)
		for i := 0; i < preloadMB*10; i++ {
			k := fmt.Sprintf("pre%06d", i)
			if err := db.Update(func(tx *buntdb.Tx) error {
				_, _, err := tx.Set(k, val, nil)
				return err
			}); err != nil {
				t.Fatal(err)
			}
		}
	}
	// warm up
	for i := 0; i < 50; i++ {
		if err := db.Update(func(tx *buntdb.Tx) error {
			_, _, err := tx.Set("warm", "up", nil)
			return err
		}); err != nil {
			t.Fatal(err)
		}
	}
	start := time.Now()
	for i := 0; i < 500; i++ {
		k := fmt.Sprintf("small%03d", i%50)
		if err := db.Update(func(tx *buntdb.Tx) error {
			_, _, err := tx.Set(k, "x", nil)
			return err
		}); err != nil {
			t.Fatal(err)
		}
	}
	return time.Since(start)
}

// V11: go.mod and go.sum of the candidate are byte-identical to the pinned
// ones. The API check is api_surface.go, which must compile.
func TestV11_NoNewDependencies(t *testing.T) {
	cand := os.Getenv("BUNTDB_VERIFY_CANDIDATE")
	if cand == "" {
		t.Skip("BUNTDB_VERIFY_CANDIDATE not set (run.sh sets it)")
	}
	for _, name := range []string{"go.mod", "go.sum"} {
		want, err := os.ReadFile(filepath.Join("pinned", name))
		if err != nil {
			t.Fatalf("pinned %s missing: %v", name, err)
		}
		got, err := os.ReadFile(filepath.Join(cand, name))
		if err != nil {
			t.Fatalf("candidate %s: %v", name, err)
		}
		norm := func(b []byte) []byte { return bytes.ReplaceAll(b, []byte("\r\n"), []byte("\n")) }
		if !bytes.Equal(norm(want), norm(got)) {
			t.Errorf("%s differs from the pinned commit (no new modules allowed)", name)
		}
	}
}
