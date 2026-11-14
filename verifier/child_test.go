package verifier

import (
	"errors"
	"fmt"
	"os"
	"testing"

	"github.com/tidwall/buntdb"
)

// TestChildMain is the body of every child process. The parent re-executes the
// test binary with -test.run=^TestChildMain$ and BUNTDB_VERIFY_CHILD set.
// In a normal run it is skipped.
func TestChildMain(t *testing.T) {
	mode := os.Getenv("BUNTDB_VERIFY_CHILD")
	if mode == "" {
		t.Skip("not a child process")
	}
	path := os.Getenv("BUNTDB_VERIFY_PATH")
	seed := SeedFromEnv()

	switch mode {
	case "open":
		// Open the file, report the state (or the error class) and the files
		// left in the directory.
		var res ChildResult
		db, err := buntdb.Open(path)
		if err != nil {
			res.Err = err.Error()
			res.ErrInvalid = errors.Is(err, buntdb.ErrInvalid)
		} else {
			st, err := ReadState(db)
			if err != nil {
				res.Err = err.Error()
			} else {
				res.State = encState(st)
			}
			if err := db.Close(); err != nil && res.Err == "" {
				res.Err = "close: " + err.Error()
			}
		}
		res.Files, _ = listDir(dirOf(path))
		emitResult(res)

	case "shrinkopen":
		// Open, Shrink, Close, Open again, report the state.
		var res ChildResult
		db, err := buntdb.Open(path)
		if err != nil {
			res.Err = "open: " + err.Error()
			res.ErrInvalid = errors.Is(err, buntdb.ErrInvalid)
			emitResult(res)
			return
		}
		if err := db.Shrink(); err != nil {
			res.Err = "shrink: " + err.Error()
			db.Close()
			emitResult(res)
			return
		}
		if err := db.Close(); err != nil {
			res.Err = "close: " + err.Error()
			emitResult(res)
			return
		}
		db, err = buntdb.Open(path)
		if err != nil {
			res.Err = "reopen: " + err.Error()
			res.ErrInvalid = errors.Is(err, buntdb.ErrInvalid)
			emitResult(res)
			return
		}
		st, err := ReadState(db)
		if err != nil {
			res.Err = err.Error()
		} else {
			res.State = encState(st)
		}
		db.Close()
		res.Files, _ = listDir(dirOf(path))
		emitResult(res)

	case "commit", "shrinkloop":
		// Commit forever under SyncPolicy Always, printing START i before each
		// Update and ACK i after it returned nil. In shrinkloop mode every
		// fifth transaction is followed by a Shrink. The parent kills us.
		db, err := OpenWith(path, buntdb.Always)
		if err != nil {
			fmt.Printf("ERR %v\n", err)
			return
		}
		g := KillGen(seed)
		for i := 1; i <= 1000000; i++ {
			fmt.Printf("START %d\n", i)
			if err := ApplyTxn(db, g.Txn(i)); err != nil {
				fmt.Printf("ERR %v\n", err)
				return
			}
			fmt.Printf("ACK %d\n", i)
			if mode == "shrinkloop" && i%5 == 0 {
				fmt.Printf("SHRINK %d\n", i)
				if err := db.Shrink(); err != nil && err != buntdb.ErrShrinkInProcess {
					fmt.Printf("ERR shrink: %v\n", err)
					return
				}
				fmt.Printf("SHRUNK %d\n", i)
			}
		}

	default:
		fmt.Printf("ERR unknown child mode %q\n", mode)
	}
}

func dirOf(path string) string {
	for i := len(path) - 1; i >= 0; i-- {
		if path[i] == '/' || path[i] == '\\' {
			return path[:i]
		}
	}
	return "."
}
