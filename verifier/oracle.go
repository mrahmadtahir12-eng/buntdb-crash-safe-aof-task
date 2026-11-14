// Package verifier holds the checks for the "crash-safe persistence for BuntDB" task.
//
// Everything here goes through BuntDB's public API only: Open, Update, View,
// Set, Delete, Shrink, SetConfig/ReadConfig, Close and ErrInvalid. Nothing
// inspects the file format.
package verifier

import (
	"fmt"
	"math/rand"
	"sort"
	"strings"
	"time"

	"github.com/tidwall/buntdb"
)

// Op is one write inside a transaction.
type Op struct {
	Key string
	Val string
	Del bool
	TTL bool
}

// Txn is one transaction: the ops are applied in order inside one Update.
type Txn struct {
	Ops []Op
}

// Gen deterministically produces transactions from a seed. Txn(i) is a pure
// function of (Seed, i), so a parent process and a child process agree on the
// expected state without sharing anything but the seed.
type Gen struct {
	Seed    int64
	NKeys   int // size of the key space
	BigAt   int // index of the transaction that also writes one oversized value (0: none)
	BigSize int // size of that value in bytes
	EmptyAt int // index of a transaction with no writes at all (0: none)
	MaxVal  int // upper bound on ordinary value sizes
}

// DefaultGen is the shape used by the file-level checks: 40 transactions,
// one empty, one carrying a value far larger than any internal buffer.
func DefaultGen(seed int64) Gen {
	return Gen{
		Seed:    seed,
		NKeys:   30,
		BigAt:   20,
		BigSize: envInt("BUNTDB_VERIFY_BIG", 1<<20+7),
		EmptyAt: 13,
		MaxVal:  200,
	}
}

// KillGen is the shape used by the process-kill checks: small values so that
// many transactions fit inside the kill window.
func KillGen(seed int64) Gen {
	return Gen{Seed: seed, NKeys: 30, MaxVal: 64}
}

func (g Gen) key(r *rand.Rand) string {
	i := r.Intn(g.NKeys)
	switch i % 7 {
	case 5:
		return fmt.Sprintf("odd\r\nkey:%02d", i)
	case 6:
		return fmt.Sprintf("key with space %02d", i)
	default:
		return fmt.Sprintf("k%02d", i)
	}
}

func (g Gen) value(r *rand.Rand) string {
	n := r.Intn(g.MaxVal + 1)
	switch r.Intn(9) {
	case 0:
		return ""
	case 1:
		return strings.Repeat("\xff", 1+r.Intn(64))
	case 2:
		return "a\x00b\x00\x00c" + randASCII(r, n)
	case 3:
		return "line1\r\nline2\r\n" + randASCII(r, n)
	case 4:
		// looks exactly like a RESP command
		return "*3\r\n$3\r\nset\r\n$2\r\nzz\r\n$5\r\nhello\r\n"
	case 5:
		// a partial RESP command
		return "*2\r\n$3\r\ndel\r\n$3\r\nk01\r\n$"
	case 6:
		// bytes that a framing scheme might use as a marker
		return "\x00\x00BTX\x01\x02" + randASCII(r, n)
	default:
		return randASCII(r, n)
	}
}

func randASCII(r *rand.Rand, n int) string {
	b := make([]byte, n)
	for i := range b {
		b[i] = byte(33 + r.Intn(94))
	}
	return string(b)
}

// Txn returns transaction i (1-based).
func (g Gen) Txn(i int) Txn {
	if i == g.EmptyAt {
		return Txn{}
	}
	r := rand.New(rand.NewSource(g.Seed*1000003 + int64(i)*7919))
	n := 1 + r.Intn(6)
	var t Txn
	for j := 0; j < n; j++ {
		op := Op{Key: g.key(r)}
		switch r.Intn(10) {
		case 0, 1:
			op.Del = true // sometimes of a key that does not exist
		case 2:
			op.Val = g.value(r)
			op.TTL = true
		default:
			op.Val = g.value(r)
		}
		t.Ops = append(t.Ops, op)
	}
	if i == g.BigAt && g.BigSize > 0 {
		t.Ops = append(t.Ops, Op{Key: "big", Val: randASCII(r, g.BigSize)})
	}
	return t
}

// ApplyToMap applies a transaction to an expected-state map.
func ApplyToMap(m map[string]string, t Txn) {
	for _, op := range t.Ops {
		if op.Del {
			delete(m, op.Key)
		} else {
			m[op.Key] = op.Val
		}
	}
}

// StateAfter returns the expected contents after transactions 1..k.
func (g Gen) StateAfter(k int) map[string]string {
	m := map[string]string{}
	for i := 1; i <= k; i++ {
		ApplyToMap(m, g.Txn(i))
	}
	return m
}

// States returns StateAfter(0), ..., StateAfter(n).
func (g Gen) States(n int) []map[string]string {
	out := make([]map[string]string, n+1)
	m := map[string]string{}
	out[0] = copyMap(m)
	for i := 1; i <= n; i++ {
		ApplyToMap(m, g.Txn(i))
		out[i] = copyMap(m)
	}
	return out
}

// ApplyTxn commits a transaction through the public API.
func ApplyTxn(db *buntdb.DB, t Txn) error {
	return db.Update(func(tx *buntdb.Tx) error {
		for _, op := range t.Ops {
			if op.Del {
				if _, err := tx.Delete(op.Key); err != nil && err != buntdb.ErrNotFound {
					return err
				}
				continue
			}
			var opts *buntdb.SetOptions
			if op.TTL {
				opts = &buntdb.SetOptions{Expires: true, TTL: 365 * 24 * time.Hour}
			}
			if _, _, err := tx.Set(op.Key, op.Val, opts); err != nil {
				return err
			}
		}
		return nil
	})
}

// ReadState returns every key and value through a read transaction.
func ReadState(db *buntdb.DB) (map[string]string, error) {
	m := map[string]string{}
	err := db.View(func(tx *buntdb.Tx) error {
		return tx.AscendKeys("*", func(k, v string) bool {
			m[k] = v
			return true
		})
	})
	return m, err
}

// SameState reports whether two states are identical.
func SameState(a, b map[string]string) bool {
	if len(a) != len(b) {
		return false
	}
	for k, v := range a {
		if w, ok := b[k]; !ok || w != v {
			return false
		}
	}
	return true
}

// MatchIndex returns the highest k such that states[k] equals s, or -1.
func MatchIndex(states []map[string]string, s map[string]string) int {
	for k := len(states) - 1; k >= 0; k-- {
		if SameState(states[k], s) {
			return k
		}
	}
	return -1
}

// DiffState describes how got differs from want, briefly.
func DiffState(want, got map[string]string) string {
	var lines []string
	for k, v := range want {
		if w, ok := got[k]; !ok {
			lines = append(lines, fmt.Sprintf("missing %q", k))
		} else if w != v {
			lines = append(lines, fmt.Sprintf("value of %q differs (want %d bytes, got %d)", k, len(v), len(w)))
		}
	}
	for k := range got {
		if _, ok := want[k]; !ok {
			lines = append(lines, fmt.Sprintf("unexpected %q", k))
		}
	}
	sort.Strings(lines)
	if len(lines) > 6 {
		lines = append(lines[:6], fmt.Sprintf("... %d more", len(lines)-6))
	}
	return strings.Join(lines, "; ")
}

func copyMap(m map[string]string) map[string]string {
	c := make(map[string]string, len(m))
	for k, v := range m {
		c[k] = v
	}
	return c
}
