package verifier

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tidwall/buntdb"
)

// Fixture is the sidecar written by fixturegen next to each .db file.
type Fixture struct {
	Seed  int64             `json:"seed"`
	N     int               `json:"n"`
	State map[string]string `json:"state"` // base64 keys and values
}

// V7: files written by the unmodified pinned code open to the same contents,
// accept new commits, reopen with old plus new, and survive a Shrink.
func TestV7_OldFilesStillOpen(t *testing.T) {
	matches, _ := filepath.Glob(filepath.Join("fixtures", "*.db"))
	if len(matches) == 0 {
		t.Fatal("no fixtures: run gen_fixtures.sh first (it builds them with the pinned upstream code)")
	}
	for _, fx := range matches {
		name := strings.TrimSuffix(filepath.Base(fx), ".db")
		t.Run(name, func(t *testing.T) {
			raw, err := os.ReadFile(strings.TrimSuffix(fx, ".db") + ".json")
			if err != nil {
				t.Fatal(err)
			}
			var f Fixture
			if err := json.Unmarshal(raw, &f); err != nil {
				t.Fatal(err)
			}
			want := decState(f.State)

			dir := t.TempDir()
			p := filepath.Join(dir, "data.db")
			if err := copyFile(fx, p); err != nil {
				t.Fatal(err)
			}
			db, err := OpenWith(p, buntdb.Always)
			if err != nil {
				t.Fatalf("old file does not open: %v", err)
			}
			got, _ := ReadState(db)
			if !SameState(want, got) {
				t.Fatalf("old file replays differently: %s", DiffState(want, got))
			}
			extra := Gen{Seed: f.Seed + 1, NKeys: 30, MaxVal: 100}
			for j := 1; j <= 5; j++ {
				tx := extra.Txn(j)
				if err := ApplyTxn(db, tx); err != nil {
					t.Fatalf("commit onto old file: %v", err)
				}
				ApplyToMap(want, tx)
			}
			if err := db.Close(); err != nil {
				t.Fatal(err)
			}
			res, err := runChild("open", p, f.Seed)
			if err != nil {
				t.Fatal(err)
			}
			if res.Err != "" {
				t.Fatalf("mixed old+new file does not reopen: %s", res.Err)
			}
			if got := decState(res.State); !SameState(want, got) {
				t.Fatalf("mixed old+new file replays differently: %s", DiffState(want, got))
			}
			res2, err := runChild("shrinkopen", p, f.Seed)
			if err != nil {
				t.Fatal(err)
			}
			if res2.Err != "" {
				t.Fatalf("Shrink of a mixed file failed: %s", res2.Err)
			}
			if got := decState(res2.State); !SameState(want, got) {
				t.Fatalf("mixed file differs after Shrink: %s", DiffState(want, got))
			}
		})
	}
}
