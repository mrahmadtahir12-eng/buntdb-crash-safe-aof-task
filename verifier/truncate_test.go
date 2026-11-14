package verifier

import (
	"math/rand"
	"os"
	"path/filepath"
	"testing"

	"github.com/tidwall/buntdb"
)

// largestK returns the largest k with sizes[k] <= cut.
func largestK(sizes []int64, cut int64) int {
	k := 0
	for i := range sizes {
		if sizes[i] <= cut {
			k = i
		}
	}
	return k
}

// sweepOffsets returns every byte offset of the file, except inside the
// oversized transaction where the boundaries plus a random sample are taken.
func sweepOffsets(sizes []int64, g Gen, r *rand.Rand) []int64 {
	n := len(sizes) - 1
	total := sizes[n]
	bigLo, bigHi := int64(-1), int64(-1)
	if g.BigAt > 0 && g.BigAt <= n {
		bigLo, bigHi = sizes[g.BigAt-1], sizes[g.BigAt]
	}
	seen := map[int64]bool{}
	var offs []int64
	add := func(o int64) {
		if o >= 0 && o <= total && !seen[o] {
			seen[o] = true
			offs = append(offs, o)
		}
	}
	for o := int64(0); o <= total; o++ {
		if o > bigLo+64 && o < bigHi-64 {
			continue
		}
		add(o)
	}
	if bigLo >= 0 && bigHi > bigLo {
		for i := 0; i < 256; i++ {
			add(bigLo + r.Int63n(bigHi-bigLo))
		}
	}
	return offs
}

// failer counts failures and stops a sweep after too many.
func failer(t *testing.T) func(format string, args ...interface{}) {
	fails := 0
	return func(format string, args ...interface{}) {
		t.Helper()
		t.Errorf(format, args...)
		fails++
		if fails > 25 {
			t.Fatalf("stopping after %d failures", fails)
		}
	}
}

// V2: cut the file at every offset; under Always the state must be exactly
// S_k for the largest k whose recorded size fits the cut.
func TestV2_TruncationSweep_Always(t *testing.T) {
	seed := SeedFromEnv()
	t.Logf("seed %d", seed)
	g := DefaultGen(seed)
	_, path, sizes := buildDB(t, buntdb.Always, g, nTxns)
	states := g.States(nTxns)
	offs := sweepOffsets(sizes, g, rand.New(rand.NewSource(seed)))
	t.Logf("file %d bytes, %d cut offsets", sizes[nTxns], len(offs))
	failf := failer(t)

	work := t.TempDir()
	p := filepath.Join(work, "data.db")
	for _, c := range offs {
		if err := writeCut(path, p, c); err != nil {
			t.Fatal(err)
		}
		k := largestK(sizes, c)
		db, err := buntdb.Open(p)
		if err != nil {
			failf("cut at %d (inside tx %d): Open failed, a torn tail must be tolerated: %v", c, k+1, err)
		} else {
			got, rerr := ReadState(db)
			db.Close()
			if rerr != nil {
				t.Fatal(rerr)
			}
			if !SameState(states[k], got) {
				if j := MatchIndex(states, got); j >= 0 {
					failf("cut at %d: expected S_%d, got S_%d", c, k, j)
				} else {
					failf("cut at %d: expected S_%d, got a state that is no S_j (partial transaction): %s", c, k, DiffState(states[k], got))
				}
			}
		}
		os.Remove(p)
	}
}

// V2 under EverySecond and Never: the state must equal some S_j.
func TestV2_TruncationSweep_OtherPolicies(t *testing.T) {
	seed := SeedFromEnv()
	failf := failer(t)
	for _, policy := range []buntdb.SyncPolicy{buntdb.EverySecond, buntdb.Never} {
		g := DefaultGen(seed + int64(policy))
		_, path, sizes := buildDB(t, policy, g, nTxns)
		states := g.States(nTxns)
		r := rand.New(rand.NewSource(seed + 7))
		all := sweepOffsets(sizes, g, r)
		var offs []int64
		for i, o := range all {
			if i%13 == 0 {
				offs = append(offs, o)
			}
		}
		for i := 0; i < 400; i++ {
			offs = append(offs, all[r.Intn(len(all))])
		}
		work := t.TempDir()
		p := filepath.Join(work, "data.db")
		for _, c := range offs {
			if err := writeCut(path, p, c); err != nil {
				t.Fatal(err)
			}
			db, err := buntdb.Open(p)
			if err != nil {
				failf("policy %d, cut at %d: Open failed: %v", policy, c, err)
			} else {
				got, rerr := ReadState(db)
				db.Close()
				if rerr != nil {
					t.Fatal(rerr)
				}
				if MatchIndex(states, got) < 0 {
					failf("policy %d, cut at %d: state is no S_j (partial transaction)", policy, c)
				}
			}
			os.Remove(p)
		}
	}
}

// V3: after a cut, three more commits, close, reopen: S_k plus those three.
// Every offset in the last three transactions, every fifth elsewhere, and a
// fresh-process reopen for every fiftieth.
func TestV3_AppendAfterCut(t *testing.T) {
	seed := SeedFromEnv()
	g := DefaultGen(seed)
	_, path, sizes := buildDB(t, buntdb.Always, g, nTxns)
	states := g.States(nTxns)
	extra := Gen{Seed: seed + 1, NKeys: 30, MaxVal: 100}
	all := sweepOffsets(sizes, g, rand.New(rand.NewSource(seed)))
	var offs []int64
	for i, o := range all {
		if o >= sizes[nTxns-3] || i%5 == 0 {
			offs = append(offs, o)
		}
	}
	t.Logf("%d cut offsets", len(offs))
	failf := failer(t)

	work := t.TempDir()
	p := filepath.Join(work, "data.db")
	for i, c := range offs {
		if err := writeCut(path, p, c); err != nil {
			t.Fatal(err)
		}
		k := largestK(sizes, c)
		want := copyMap(states[k])
		db, err := OpenWith(p, buntdb.Always)
		if err != nil {
			failf("cut at %d: Open failed: %v", c, err)
			os.Remove(p)
			continue
		}
		ok := true
		for j := 1; j <= 3; j++ {
			tx := extra.Txn(j)
			if err := ApplyTxn(db, tx); err != nil {
				failf("cut at %d: commit after cut failed: %v", c, err)
				ok = false
				break
			}
			ApplyToMap(want, tx)
		}
		db.Close()
		if ok {
			db, err = buntdb.Open(p)
			if err != nil {
				failf("cut at %d: reopen after appending failed: %v", c, err)
			} else {
				got, _ := ReadState(db)
				db.Close()
				if !SameState(want, got) {
					failf("cut at %d: after appending 3 commits, reopen differs: %s", c, DiffState(want, got))
				}
			}
			if i%50 == 0 {
				res, err := runChild("open", p, seed)
				if err != nil {
					t.Fatal(err)
				}
				if res.Err != "" {
					failf("cut at %d: fresh process reopen failed: %s", c, res.Err)
				} else if got := decState(res.State); !SameState(want, got) {
					failf("cut at %d: fresh process state differs: %s", c, DiffState(want, got))
				}
			}
		}
		os.Remove(p)
	}
}
