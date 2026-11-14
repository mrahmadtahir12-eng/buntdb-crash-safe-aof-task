// fixturegen writes V7 fixtures with whatever buntdb this module resolves to.
// Its go.mod pins that to the unmodified upstream checkout, so the fixtures
// come from the original code, never from a candidate.
//
// usage: go run . <outdir>
package main

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	verifier "buntdbverify"

	"github.com/tidwall/buntdb"
)

func main() {
	if len(os.Args) != 2 {
		fmt.Fprintln(os.Stderr, "usage: fixturegen <outdir>")
		os.Exit(2)
	}
	out := os.Args[1]
	if err := os.MkdirAll(out, 0o755); err != nil {
		panic(err)
	}
	for _, seed := range []int64{101, 102, 103, 104, 105} {
		g := verifier.DefaultGen(seed)
		g.BigSize = 64 * 1024 // keep fixtures small; still larger than any internal buffer
		n := 40
		path := filepath.Join(out, fmt.Sprintf("seed%d.db", seed))
		os.Remove(path)
		db, err := verifier.OpenWith(path, buntdb.Always)
		if err != nil {
			panic(err)
		}
		for i := 1; i <= n; i++ {
			if err := verifier.ApplyTxn(db, g.Txn(i)); err != nil {
				panic(err)
			}
		}
		if seed%2 == 0 {
			// half the fixtures have been shrunk by the old code
			if err := db.Shrink(); err != nil {
				panic(err)
			}
			for i := n + 1; i <= n+5; i++ {
				if err := verifier.ApplyTxn(db, g.Txn(i)); err != nil {
					panic(err)
				}
			}
			n += 5
		}
		state, err := verifier.ReadState(db)
		if err != nil {
			panic(err)
		}
		if err := db.Close(); err != nil {
			panic(err)
		}
		enc := map[string]string{}
		for k, v := range state {
			enc[base64.StdEncoding.EncodeToString([]byte(k))] = base64.StdEncoding.EncodeToString([]byte(v))
		}
		b, _ := json.MarshalIndent(map[string]interface{}{"seed": seed, "n": n, "state": enc}, "", " ")
		if err := os.WriteFile(filepath.Join(out, fmt.Sprintf("seed%d.json", seed)), b, 0o644); err != nil {
			panic(err)
		}
		fi, _ := os.Stat(path)
		fmt.Printf("wrote %s (%d bytes, %d keys)\n", path, fi.Size(), len(state))
	}
}
