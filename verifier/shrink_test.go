package verifier

import (
	"math/rand"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/tidwall/buntdb"
)

// V8: one goroutine commits while another calls Shrink, with random timing;
// after both finish, close and reopen: the state must equal the oracle.
// run.sh passes -race.
func TestV8_ShrinkWithConcurrentWrites(t *testing.T) {
	seed := SeedFromEnv()
	t.Logf("seed %d", seed)
	rounds := envInt("BUNTDB_VERIFY_SHRINK_ROUNDS", 50)
	r := rand.New(rand.NewSource(seed + 8))
	for round := 0; round < rounds; round++ {
		g := Gen{Seed: seed + int64(round)*101, NKeys: 40, MaxVal: 300}
		n := 60 + r.Intn(200)
		dir := t.TempDir()
		path := filepath.Join(dir, "data.db")
		db, err := OpenWith(path, buntdb.SyncPolicy(r.Intn(3)))
		if err != nil {
			t.Fatal(err)
		}
		var wg sync.WaitGroup
		done := make(chan struct{})
		var commitErr, shrinkErr error
		wg.Add(2)
		go func() {
			defer wg.Done()
			defer close(done)
			for i := 1; i <= n; i++ {
				if err := ApplyTxn(db, g.Txn(i)); err != nil {
					commitErr = err
					return
				}
				if i%7 == 0 {
					time.Sleep(time.Duration(rand.Intn(3)) * time.Millisecond)
				}
			}
		}()
		go func() {
			defer wg.Done()
			for {
				select {
				case <-done:
					return
				default:
				}
				if err := db.Shrink(); err != nil && err != buntdb.ErrShrinkInProcess {
					shrinkErr = err
					return
				}
				time.Sleep(time.Duration(rand.Intn(20)) * time.Millisecond)
			}
		}()
		wg.Wait()
		if commitErr != nil {
			t.Fatalf("round %d: commit failed: %v", round, commitErr)
		}
		if shrinkErr != nil {
			t.Fatalf("round %d: Shrink failed: %v", round, shrinkErr)
		}
		// one more Shrink after the writes, then more writes, then close
		if err := db.Shrink(); err != nil {
			t.Fatalf("round %d: final Shrink: %v", round, err)
		}
		want := g.StateAfter(n)
		for i := n + 1; i <= n+3; i++ {
			tx := g.Txn(i)
			if err := ApplyTxn(db, tx); err != nil {
				t.Fatal(err)
			}
			ApplyToMap(want, tx)
		}
		if err := db.Close(); err != nil {
			t.Fatal(err)
		}
		db, err = buntdb.Open(path)
		if err != nil {
			t.Fatalf("round %d: reopen: %v", round, err)
		}
		got, _ := ReadState(db)
		db.Close()
		if !SameState(want, got) {
			t.Fatalf("round %d (%d txns): state after Shrink alongside writes differs: %s", round, n, DiffState(want, got))
		}
		names, _ := listDir(dir)
		if len(names) != 1 {
			t.Fatalf("round %d: after a clean close the directory holds %v", round, names)
		}
	}
}
