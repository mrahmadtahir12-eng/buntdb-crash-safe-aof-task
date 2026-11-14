# Verifiers

Status (2026-11-15): every check below exists as Go test code in the task's verifier module
(`run.sh` builds the clean tree and runs it). It has been run once on the unmodified checkout and
once on a reference solution, both in quick mode on Windows (seed 424242, oversized value 70 KB,
20 kill runs, 15 Shrink-kill runs, 10 Shrink rounds, no race detector). The "Baseline" column is
that measurement. The unmodified repo fails V2, V3, V5 and V6, each for its stated reason; the
reference passes every check. The full-size sweep under -race on Linux is still to do.

## Harness design (applies to every check)
- The verifier builds a clean tree: the candidate's non-test Go sources, the verifier's own pinned
  `go.mod` and `go.sum`, and the verifier's tests. Candidate `_test.go` files are ignored.
- Checks use only the public API: Open, Update, View, Set, Delete, Shrink, ReadConfig, SetConfig,
  Close, Begin/Rollback and ErrInvalid. Nothing looks at the file format or private names.
- A seeded generator produces 40 transactions: multi-key sets, deletes (some of missing keys),
  overwrites of the same keys, keys with CRLF and spaces, empty values, values with CRLF, NUL
  bytes, runs of 0xFF, text that is exactly a RESP command, text that is a partial RESP command,
  bytes that look like a frame marker, a value with a one-year TTL, one transaction with no writes
  at all, and one value of about 1 MB (3 MB in the full run), larger than any internal buffer.
- The oracle holds the expected state S_k after each transaction k, and the file size right after
  each Update returned. Expected states are a pure function of the seed, so a child process can
  recompute them.
- The seed is chosen at verification time and is not in the repository.
- Crash, kill and corruption checks reopen from a copy of only the database file (or, for Shrink,
  of the whole directory) in a fresh empty directory. The kill checks and samples of the others do
  this in a fresh process, by re-executing the test binary in a child mode. A solution that hides
  state in a side file or in process memory fails there.

## Checks

R1 to R7 are the seven bullets of the prompt, in order; "opening sentence" and "last paragraph" are
the two sentences around them.

| ID | Prompt sentence | What it does | Why a correct solution passes | Baseline (unmodified repo, measured) |
|---|---|---|---|---|
| V1a | opening sentence ("made crash-safe without breaking the public API or anyone's existing data files") | Write 40 transactions, close, reopen in the same process and in a fresh process, compare to S_40. | Public API only; the plain round trip any correct solution must keep. | pass |
| V1b | R2, "A rolled-back transaction leaves no trace" | Begin a writable transaction, set and delete, roll it back; run an Update whose function returns an error. Reopen: neither change is present. | Observed through Get and the reopened state only. | pass |
| V1c | last paragraph, "Keep the exported API and in-memory behaviour as they are" | Run the upstream test suite from the pinned commit against the candidate sources. | No upstream test asserts bytes of a file the library wrote itself; the raw-RESP tests write legacy files, which R5 requires to keep opening, so nothing is removed from the suite. | pass. One upstream test (a zero-TTL item expected to be gone at once) fails on Windows on the unmodified code too, three runs out of three: clock resolution, not persistence |
| V2 | R1 | Truncation sweep: cut a copy at every byte offset (sampled inside the oversized value) and open it. Under Always the state must equal S_k for the largest k whose recorded size fits the cut. Under EverySecond and Never (sampled offsets) the state must equal some S_j. | Exact match is only demanded under Always, where "Update returned nil" means the bytes are in the file. The other policies may buffer, so only atomicity is demanded. Open must succeed on a torn tail. | FAIL, both policies: the first cut inside the first transaction (offset 68) already opens to a state that is no S_j, with one key of that transaction applied and the rest missing |
| V3 | R3 | After a cut (every offset in the last three transactions, every fifth elsewhere), commit three more, close, reopen in-process; every fiftieth also in a fresh process. State must be S_k plus those three. | Catches truncating the torn tail but leaving or ignoring garbage after it. Format-agnostic. | FAIL: the appends themselves work (the old loader truncates to the last whole command), but the half-applied state from the cut carries through, so the reopened state is not S_k plus three |
| V4 | R2 | A child process commits under Always and prints START i before each Update and ACK i after it returns nil. The parent kills it (SIGKILL / TerminateProcess) after a random 5-400 ms, 100 runs. The file alone, in a fresh directory and process, must open to S_k with acks <= k <= started. | kill -9 cannot test fsync, only that the write happened before Update returned. That is exactly what the sentence promises. | pass (20 runs): a single write() completes even if the process dies right after. This check guards against a solution that acks before writing |
| V5 | R3 | For each byte of the last transaction, three mutations (XOR 0xFF, one-bit flip, zero): Open must succeed with S_39, three more commits must append, and a reopen (in-process; every 97th also in a fresh process) must show S_39 plus those three. | Derived only from "any byte altered". The last transaction is small and not the empty one, so the byte range is well defined. | FAIL: the very first byte of the last transaction, altered, makes Open return ErrInvalid instead of discarding; payload bytes load a wrong value |
| V6 | R4 | For each byte of each earlier transaction (sampled inside the oversized value), the same three mutations: Open must return an error with errors.Is(err, ErrInvalid), the file hash must be unchanged, and no file may be added to the directory. | The sentence says any byte. A format with an unchecked "reserved" byte fails, and the prompt makes that a fair failure. Altering length bytes in the middle is what catches a reader that mistakes corruption for a torn tail. | FAIL: of 34,380 altered files, 24,596 opened without complaint (altered key or value bytes load silently, overrunning lengths truncate later data) and 315 gave an error that is not ErrInvalid |
| V7 | R5 | Fixture files generated by the unmodified pinned code when the verifier is built (five seeds, half of them shrunk by the old code). Open each (same contents), append five transactions, close, reopen in a fresh process (old plus new), then Shrink and reopen (unchanged). | Fixtures come from the original code, through a separate module whose go.mod pins buntdb to the upstream checkout, never from the solution. | pass |
| V8 | R6 | One goroutine commits 60-260 transactions while another calls Shrink in a loop with random pauses, 50 rounds, each under a random SyncPolicy, run with -race. A final Shrink, three more commits, close, reopen: the state must equal the oracle, and the directory must hold one file. | Overwrites of the same keys mean a lost, duplicated or reordered transaction brings back an older value, so the state comparison catches it. | pass (10 rounds): the old tail copy runs under the lock. Guards the new Shrink |
| V9 | R6 | A child commits under Always and calls Shrink after every fifth transaction, printing acks. SIGKILL after a random 20-1500 ms, 60 runs. The whole directory (leftovers included) is copied and opened in a fresh process: S_k with acks <= k <= started. Then Shrink again and reopen in another fresh process: the state is unchanged. | No knowledge of the implementation is needed. Random kills cover the snapshot write, the tail copy, the swap and the reopen. The syscall-targeted kills (strace at the Nth rename, unlink, write, fsync) described in the plan are not in the harness yet; random kills with more runs are the fallback the plan allows. | pass (15 runs, Windows): the old swap is a close, rename, reopen and leftovers are harmless. Guards the new Shrink; on Linux the rename is atomic, on Windows the remove-then-rename fallback has a window with no file that 15 runs did not hit |
| V10a | R7, "The database remains one file at the path given to Open" | After a clean close the directory holds exactly one file. | A directory listing; no format knowledge. | pass |
| V10b | R7, "the cost of a commit doesn't grow with the size of the database" | Time 500 small commits on a tiny database and on one of about 50 MB (SyncPolicy Never so fsync does not dominate): the ratio must be under 20. | Appends pass easily. Rewriting the whole file per commit does not, and the prompt states this constraint. | pass: ratio 0.6 on the unmodified code, 1.1 on the reference (tiny vs 10 MB in quick mode) |
| V11a | last paragraph, "add no dependencies beyond the standard library and what go.mod already has" | go.mod and go.sum are byte-identical to the pinned commit. | Pure surface check. | pass |
| V11b | last paragraph, "Keep the exported API" | A file in the verifier assigns every exported function, method, error, constant and option struct to its pinned signature, so a changed or removed export fails to compile. | Pure surface check; a solution may add exports, only not change or remove one. | pass |

Grouping: V1a-c, V7, V10a-b and V11a-b are the basics. V2 to V6, V8 and V9 are the hard group. Reward is all
or nothing on the hard group. The basics are reported separately so a failed attempt still shows how
far it got. In the harness the split rows share a test function: V1a and V1b are TestV1_RoundTripAndRollback
and V1c is the upstream-suite step of run.sh; V10a-b are TestV10_OneFileAndCommitCost; V11a is
TestV11_NoNewDependencies and V11b is api_surface.go, which must compile.

## Edge cases the generator and checks cover
- A file of zero bytes (cut at offset 0), a file holding only part of the first transaction, a cut
  exactly on every transaction boundary, and a transaction with no writes (its byte range is empty).
- A single transaction larger than any internal buffer (about 1 MB by default, 3 MB in the full
  run, via BUNTDB_VERIFY_BIG).
- Values that imitate framing bytes, that are exactly a RESP command, or a partial one, so
  resync-by-scanning approaches get fooled.
- Alteration of every byte in the middle, which includes any length or header bytes a format has,
  and so catches a corrupt record that is made to look like a torn tail.
- Leftover temp files before the next Open and before the next Shrink (V9 copies the whole
  directory and then shrinks again).
- Keys with CRLF and spaces, values with NUL and 0xFF, and a TTL item, so the old loader's
  "ae" path is replayed too.

## How each way of gaming is resisted
- **Do nothing / unmodified repo:** fails V2, V3, V5 and V6, each for its own stated reason
  (half-applied transactions, carried into the append check; wrong error on the last
  transaction; silent acceptance of altered payload bytes). The baseline run records which check
  fails and why, because a baseline that fails for the wrong reason proves nothing.
- **Disable persistence or keep state in memory or a side file:** reopening in a fresh process
  from a copy of only the database file defeats it (V1a, V3, V4, V5, V7), and the one-file check
  (V8, V10a) catches side files.
- **Detect the test environment, or special-case keys and values:** seeds and data are chosen at
  verification time; run.sh greps the candidate for os.Getenv, testing.Testing, runtime.Caller and
  flag.Lookup("test...") used to branch. This is a screen, not proof.
- **Edit the verifier or add dependencies:** the verifier owns go.mod, go.sum and all test code;
  candidate test files are ignored; V11a compares go.mod and go.sum byte for byte.
- **Rewrite the file on every commit:** V10b.
- **Return success without writing:** V4 kills the process after the ack.
- **Truncate on any doubt:** V6 demands ErrInvalid and an unchanged file for every altered byte
  before the last transaction.

## Validation of the verifiers themselves
1. DONE (2026-11-15, Windows, quick mode, seed 424242): `gen_fixtures.sh`, then
   `run.sh ../upstream/buntdb` on the unmodified checkout. V2 (both policies), V3, V5 and V6
   fail, each for the reason in the Baseline column; V1a-c, V4, V7, V8, V9, V10a-b, V11a-b pass. Log:
   notes/baseline_quick.log.
2. DONE, same settings: the reference solution (`reference/apply.py`: per-transaction frames with
   a length, a body checksum and a header checksum, torn-tail truncation, a sequence-chained
   resync that a value cannot imitate, and a Shrink that writes a temp file, re-frames the tail,
   fsyncs and renames) passes every check. Log: notes/reference_quick.log. The upstream
   suite passes against it except the one Windows clock-resolution test that also fails on the
   unmodified code.
3. DONE by 1 and 2 together: each negative has its positive twin (V2 and V3 turn green once
   commits are framed and the truncation is physical; V5 and V6 once the tail/middle rule is
   right).
4. TO DO: the full sweep (BUNTDB_VERIFY_BIG=3000000), the kill checks with several hundred runs,
   and -race, on Linux. The quick run took 12 minutes for the baseline and 19 for the reference
   on Windows, where V6 alone was 12 minutes because of per-file overhead; expect Linux to be
   much faster.

Honest limit: the checks are proven in quick mode on one machine. The race detector, the
multi-megabyte value and the syscall-targeted Shrink kills are not yet exercised. Say so on the
video.
