package verifier

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/tidwall/buntdb"
)

func envInt(name string, def int) int {
	if s := os.Getenv(name); s != "" {
		if v, err := strconv.Atoi(s); err == nil {
			return v
		}
	}
	return def
}

// SeedFromEnv returns BUNTDB_VERIFY_SEED, or a seed drawn now. The seed is
// chosen at verification time and is never stored in the repository.
func SeedFromEnv() int64 {
	if s := os.Getenv("BUNTDB_VERIFY_SEED"); s != "" {
		if v, err := strconv.ParseInt(s, 10, 64); err == nil {
			return v
		}
	}
	return time.Now().UnixNano() % 1000000007
}

// OpenWith opens a database and applies a sync policy with auto-shrink off.
func OpenWith(path string, policy buntdb.SyncPolicy) (*buntdb.DB, error) {
	db, err := buntdb.Open(path)
	if err != nil {
		return nil, err
	}
	var cfg buntdb.Config
	if err := db.ReadConfig(&cfg); err != nil {
		db.Close()
		return nil, err
	}
	cfg.SyncPolicy = policy
	cfg.AutoShrinkDisabled = true
	if err := db.SetConfig(cfg); err != nil {
		db.Close()
		return nil, err
	}
	return db, nil
}

func fileSize(path string) (int64, error) {
	fi, err := os.Stat(path)
	if err != nil {
		return 0, err
	}
	return fi.Size(), nil
}

func fileSHA(path string) (string, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:]), nil
}

func copyFile(src, dst string) error {
	b, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	return os.WriteFile(dst, b, 0o644)
}

func copyDir(src, dst string) error {
	entries, err := os.ReadDir(src)
	if err != nil {
		return err
	}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		if err := copyFile(filepath.Join(src, e.Name()), filepath.Join(dst, e.Name())); err != nil {
			return err
		}
	}
	return nil
}

func listDir(dir string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	var names []string
	for _, e := range entries {
		names = append(names, e.Name())
	}
	sort.Strings(names)
	return names, nil
}

// writeCut writes the first n bytes of src to dst.
func writeCut(src, dst string, n int64) error {
	b, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	if n > int64(len(b)) {
		n = int64(len(b))
	}
	return os.WriteFile(dst, b[:n], 0o644)
}

// Mutations applied to one byte, in order: XOR 0xFF, one-bit flip, zero.
const numMutations = 3

func mutateByte(b byte, off int64, m int) byte {
	switch m {
	case 0:
		return b ^ 0xff
	case 1:
		return b ^ (1 << uint(off%8))
	default:
		if b == 0 {
			return 1
		}
		return 0
	}
}

// writeMutated copies src to dst with byte off altered by mutation m.
func writeMutated(src, dst string, off int64, m int) error {
	b, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	if off < 0 || off >= int64(len(b)) {
		return fmt.Errorf("offset %d outside file of %d bytes", off, len(b))
	}
	c := make([]byte, len(b))
	copy(c, b)
	c[off] = mutateByte(c[off], off, m)
	return os.WriteFile(dst, c, 0o644)
}

func mutationName(m int) string {
	return [...]string{"xor 0xff", "bit flip", "zero"}[m]
}

// ---- cross-process helpers ------------------------------------------------

// ChildResult is what a child process reports, as a line starting with "@@RESULT ".
type ChildResult struct {
	Err        string            `json:"err"`
	ErrInvalid bool              `json:"errinvalid"`
	State      map[string]string `json:"state"` // base64 keys and values
	Files      []string          `json:"files"`
}

func encState(m map[string]string) map[string]string {
	out := make(map[string]string, len(m))
	for k, v := range m {
		out[base64.StdEncoding.EncodeToString([]byte(k))] = base64.StdEncoding.EncodeToString([]byte(v))
	}
	return out
}

func decState(m map[string]string) map[string]string {
	out := make(map[string]string, len(m))
	for k, v := range m {
		kb, _ := base64.StdEncoding.DecodeString(k)
		vb, _ := base64.StdEncoding.DecodeString(v)
		out[string(kb)] = string(vb)
	}
	return out
}

func emitResult(r ChildResult) {
	b, _ := json.Marshal(r)
	fmt.Printf("@@RESULT %s\n", b)
}

// childCommand builds a command that re-executes this test binary in child
// mode. The child links the same candidate code but shares nothing else with
// the parent: no memory, no open files.
func childCommand(mode, path string, seed int64) *exec.Cmd {
	cmd := exec.Command(os.Args[0], "-test.run=^TestChildMain$", "-test.timeout=0")
	cmd.Env = append(os.Environ(),
		"BUNTDB_VERIFY_CHILD="+mode,
		"BUNTDB_VERIFY_PATH="+path,
		"BUNTDB_VERIFY_SEED="+strconv.FormatInt(seed, 10),
	)
	cmd.Dir = filepath.Dir(path)
	return cmd
}

// runChild runs a child to completion and parses its result line.
func runChild(mode, path string, seed int64) (ChildResult, error) {
	cmd := childCommand(mode, path, seed)
	out, err := cmd.CombinedOutput()
	res, perr := parseResult(out)
	if perr != nil {
		if err != nil {
			return res, fmt.Errorf("child %s: %v: %s", mode, err, trim(out))
		}
		return res, perr
	}
	return res, nil
}

func parseResult(out []byte) (ChildResult, error) {
	var res ChildResult
	for _, line := range bytes.Split(out, []byte("\n")) {
		if bytes.HasPrefix(line, []byte("@@RESULT ")) {
			if err := json.Unmarshal(bytes.TrimPrefix(line, []byte("@@RESULT ")), &res); err != nil {
				return res, err
			}
			return res, nil
		}
	}
	return res, fmt.Errorf("no result line in child output: %s", trim(out))
}

func trim(b []byte) string {
	s := string(b)
	if len(s) > 800 {
		s = s[:800] + "..."
	}
	return s
}

// runKillChild starts a child in a commit loop, kills it after the delay, and
// returns the highest START and ACK counters it printed before dying.
func runKillChild(mode, path string, seed int64, after time.Duration) (maxStart, maxAck int, err error) {
	cmd := childCommand(mode, path, seed)
	var buf bytes.Buffer
	cmd.Stdout = &buf
	cmd.Stderr = io.Discard
	if err := cmd.Start(); err != nil {
		return 0, 0, err
	}
	time.Sleep(after)
	_ = cmd.Process.Kill()
	_ = cmd.Wait()
	for _, line := range strings.Split(buf.String(), "\n") {
		f := strings.Fields(line)
		if len(f) != 2 {
			continue
		}
		n, convErr := strconv.Atoi(f[1])
		if convErr != nil {
			continue
		}
		switch f[0] {
		case "START":
			if n > maxStart {
				maxStart = n
			}
		case "ACK":
			if n > maxAck {
				maxAck = n
			}
		case "ERR":
			return maxStart, maxAck, fmt.Errorf("child reported an error: %s", line)
		}
	}
	return maxStart, maxAck, nil
}
