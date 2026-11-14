package verifier

import (
	"math/rand"
	"path/filepath"
	"testing"
	"time"
)

// findK returns a k in [lo, hi] whose expected state equals got, or -1.
func findK(g Gen, got map[string]string, lo, hi int) int {
	m := g.StateAfter(lo)
	if SameState(m, got) {
		return lo
	}
	for k := lo + 1; k <= hi; k++ {
		ApplyToMap(m, g.Txn(k))
		if SameState(m, got) {
			return k
		}
	}
	return -1
}

// V4: a child commits under Always and acks after each Update. Killed at a
// random moment, the file reopened alone in a fresh process must show S_k
// with acks <= k <= started.
func TestV4_KillAfterAck(t *testing.T) {
	seed := SeedFromEnv()
	t.Logf("seed %d", seed)
	runs := envInt("BUNTDB_VERIFY_KILL_RUNS", 100)
	r := rand.New(rand.NewSource(seed))
	g := KillGen(seed)
	skipped := 0
	for run := 0; run < runs; run++ {
		dir := t.TempDir()
		path := filepath.Join(dir, "data.db")
		delay := time.Duration(5+r.Intn(400)) * time.Millisecond
		maxStart, maxAck, err := runKillChild("commit", path, seed, delay)
		if err != nil {
			t.Fatalf("run %d: %v", run, err)
		}
		if maxStart == 0 {
			skipped++
			continue
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
			t.Errorf("run %d (killed after %v, acks %d, started %d): fresh Open failed: %s", run, delay, maxAck, maxStart, res.Err)
			continue
		}
		got := decState(res.State)
		if k := findK(g, got, maxAck, maxStart); k < 0 {
			t.Errorf("run %d (killed after %v): state matches no S_k with %d <= k <= %d", run, delay, maxAck, maxStart)
		}
	}
	t.Logf("%d runs, %d killed before the first transaction", runs, skipped)
}

// V9: a child commits and shrinks in a loop; killed at random moments. The
// whole directory (leftovers included) is reopened in a fresh process: S_k with
// acks <= k <= started. Then Shrink again and reopen: unchanged.
func TestV9_KillDuringShrink(t *testing.T) {
	seed := SeedFromEnv()
	t.Logf("seed %d", seed)
	runs := envInt("BUNTDB_VERIFY_SHRINK_KILL_RUNS", 60)
	r := rand.New(rand.NewSource(seed + 9))
	g := KillGen(seed)
	for run := 0; run < runs; run++ {
		dir := t.TempDir()
		path := filepath.Join(dir, "data.db")
		delay := time.Duration(20+r.Intn(1500)) * time.Millisecond
		maxStart, maxAck, err := runKillChild("shrinkloop", path, seed, delay)
		if err != nil {
			t.Fatalf("run %d: %v", run, err)
		}
		if maxStart == 0 {
			continue
		}
		fresh := t.TempDir()
		if err := copyDir(dir, fresh); err != nil {
			t.Fatal(err)
		}
		p := filepath.Join(fresh, "data.db")
		res, err := runChild("open", p, seed)
		if err != nil {
			t.Fatal(err)
		}
		if res.Err != "" {
			t.Errorf("run %d (killed after %v, files %v): fresh Open failed: %s", run, delay, res.Files, res.Err)
			continue
		}
		got := decState(res.State)
		k := findK(g, got, maxAck, maxStart)
		if k < 0 {
			t.Errorf("run %d (killed after %v, files %v): state matches no S_k with %d <= k <= %d", run, delay, res.Files, maxAck, maxStart)
			continue
		}
		res2, err := runChild("shrinkopen", p, seed)
		if err != nil {
			t.Fatal(err)
		}
		if res2.Err != "" {
			t.Errorf("run %d: Shrink after the crash failed: %s", run, res2.Err)
			continue
		}
		if got2 := decState(res2.State); !SameState(got, got2) {
			t.Errorf("run %d: state changed across the Shrink after the crash: %s", run, DiffState(got, got2))
		}
	}
}
