package verifier

import (
	"errors"
	"math/rand"
	"os"
	"path/filepath"
	"testing"

	"github.com/tidwall/buntdb"
)

// V5: every byte of the last transaction, three mutations each: Open succeeds
// with S_(N-1), later commits append and survive a reopen.
func TestV5_LastTransactionAltered(t *testing.T) {
	seed := SeedFromEnv()
	t.Logf("seed %d", seed)
	g := DefaultGen(seed)
	_, path, sizes := buildDB(t, buntdb.Always, g, nTxns)
	states := g.States(nTxns)
	extra := Gen{Seed: seed + 2, NKeys: 30, MaxVal: 100}
	lo, hi := sizes[nTxns-1], sizes[nTxns]
	if hi <= lo {
		t.Fatalf("last transaction wrote nothing (%d..%d); generator shape is wrong", lo, hi)
	}
	t.Logf("last transaction occupies bytes %d..%d", lo, hi)
	failf := failer(t)

	work := t.TempDir()
	p := filepath.Join(work, "data.db")
	count := 0
	for off := lo; off < hi; off++ {
		for m := 0; m < numMutations; m++ {
			if err := writeMutated(path, p, off, m); err != nil {
				t.Fatal(err)
			}
			db, err := OpenWith(p, buntdb.Always)
			if err != nil {
				failf("byte %d %s: Open must discard the last transaction, got error: %v", off, mutationName(m), err)
			} else {
				got, _ := ReadState(db)
				if !SameState(states[nTxns-1], got) {
					if j := MatchIndex(states, got); j >= 0 {
						failf("byte %d %s: expected S_%d, got S_%d", off, mutationName(m), nTxns-1, j)
					} else {
						failf("byte %d %s: expected S_%d, got a state that is no S_j: %s", off, mutationName(m), nTxns-1, DiffState(states[nTxns-1], got))
					}
				}
				want := copyMap(states[nTxns-1])
				for j := 1; j <= 3; j++ {
					tx := extra.Txn(j)
					if err := ApplyTxn(db, tx); err != nil {
						failf("byte %d %s: commit after discard failed: %v", off, mutationName(m), err)
					}
					ApplyToMap(want, tx)
				}
				db.Close()
				db, err = buntdb.Open(p)
				if err != nil {
					failf("byte %d %s: reopen after appending failed: %v", off, mutationName(m), err)
				} else {
					got, _ := ReadState(db)
					db.Close()
					if !SameState(want, got) {
						failf("byte %d %s: after appending, reopen differs: %s", off, mutationName(m), DiffState(want, got))
					}
				}
				if count%97 == 0 {
					res, err := runChild("open", p, seed)
					if err != nil {
						t.Fatal(err)
					}
					if res.Err != "" {
						failf("byte %d %s: fresh process reopen failed: %s", off, mutationName(m), res.Err)
					} else if got := decState(res.State); !SameState(want, got) {
						failf("byte %d %s: fresh process state differs: %s", off, mutationName(m), DiffState(want, got))
					}
				}
			}
			count++
			os.Remove(p)
		}
	}
	t.Logf("%d mutated files opened", count)
}

// V6: every byte of every earlier transaction (sampled inside the oversized
// one), three mutations each: Open returns an error with errors.Is(err,
// ErrInvalid), the file hash is unchanged, no file was added.
func TestV6_EarlierTransactionAltered(t *testing.T) {
	seed := SeedFromEnv()
	t.Logf("seed %d", seed)
	g := DefaultGen(seed)
	_, path, sizes := buildDB(t, buntdb.Always, g, nTxns)
	r := rand.New(rand.NewSource(seed + 3))
	bigLo, bigHi := sizes[g.BigAt-1], sizes[g.BigAt]
	var offs []int64
	for o := int64(0); o < sizes[nTxns-1]; o++ {
		if o > bigLo+64 && o < bigHi-64 {
			continue
		}
		offs = append(offs, o)
	}
	for i := 0; i < 400; i++ {
		offs = append(offs, bigLo+r.Int63n(bigHi-bigLo))
	}
	t.Logf("%d offsets x %d mutations", len(offs), numMutations)
	failf := failer(t)

	work := t.TempDir()
	p := filepath.Join(work, "data.db")
	silent, wrongErr, changed := 0, 0, 0
	for _, off := range offs {
		for m := 0; m < numMutations; m++ {
			if err := writeMutated(path, p, off, m); err != nil {
				t.Fatal(err)
			}
			before, _ := fileSHA(p)
			db, err := buntdb.Open(p)
			switch {
			case err == nil:
				db.Close()
				silent++
				if silent <= 10 {
					failf("byte %d %s: Open succeeded; corruption in the middle must be reported", off, mutationName(m))
				}
			case !errors.Is(err, buntdb.ErrInvalid):
				wrongErr++
				if wrongErr <= 10 {
					failf("byte %d %s: error is not ErrInvalid: %v", off, mutationName(m), err)
				}
			}
			after, _ := fileSHA(p)
			if before != after {
				changed++
				if changed <= 10 {
					failf("byte %d %s: Open changed the file", off, mutationName(m))
				}
			}
			names, _ := listDir(work)
			if len(names) != 1 {
				failf("byte %d %s: Open left extra files: %v", off, mutationName(m), names)
			}
			os.Remove(p)
		}
	}
	if silent+wrongErr+changed > 0 {
		t.Fatalf("summary: %d silent opens, %d wrong errors, %d files changed (of %d trials)", silent, wrongErr, changed, len(offs)*numMutations)
	}
}
