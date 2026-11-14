# BuntDB persistence on the pinned commit: what the code does today

Pinned commit: 417eb7c4c8b654adf2b56784d18c6a80136d445a (2026-11-07, "Merge pull request #106").
Licence: MIT (LICENSE file, Josh Baker 2016). Module github.com/tidwall/buntdb, go 1.18.
All persistence lives in buntdb.go (2427 lines). Line numbers below are on that commit.

## File format
- Redis AOF style: a sequence of RESP arrays, one per command. Only SET (3 or 5 parts, "ex"/"ae"
  TTL), DEL (2 parts) and FLUSHDB (1 part) are accepted (readLoad, 810-986).
- No transaction boundary exists on disk. A commit with k changes is k commands.
- NUL bytes between commands are skipped (823-831). Note: the skipped NULs are added to the named
  return `n`, but readLoad returns `totalSize`, so they are not counted in the truncation offset.

## Write path (Tx.Commit, 1219-1283)
- All commands of the transaction are appended to db.buf and written with ONE file.Write (1250).
- A short write (err != nil, n > 0) seeks back n bytes and truncates, then rolls back in memory.
- SyncPolicy Always: file.Sync() after the write, error ignored (1270-1272).
- EverySecond: backgroundManager syncs once a second if flushes changed (620-628).
- Rollback never touches the file (1285-1302).

## Load path (load, 988-1019; readLoad, 810-986)
- Any structural problem returns ErrInvalid: first byte not '*', line not ending in \r\n, a non-digit
  in a count, a missing '$', a payload not followed by \r\n, wrong arity, unknown command.
- io.EOF inside a command becomes io.ErrUnexpectedEOF (811-815); load() then truncates the file to
  totalSize, the end of the last whole COMMAND (997-1002), and seeks there. So a torn tail is
  handled, but at command granularity, not transaction granularity.
- A payload byte that is altered is NOT detected: the key or value just loads wrong.
- A corrupted length digit that makes a command run past EOF produces ErrUnexpectedEOF, and load()
  truncates everything from that command on. That is the "opened without complaint but missing
  recent writes" incident: corruption in the middle treated as a torn tail.

## Shrink (650-788)
- Records endpos under the lock, releases it, sleeps 250ms, creates <path>.tmp with os.Create.
- Writes a SET per live item in chunks of ~1000 items under RLock (writeSetTo with "ae" absolute
  expiry), then under the full lock copies bytes [endpos, EOF) of the live file into the tmp file,
  f.Sync(), closes both, renameFile(tmp, path) (panics on failure), reopens, records lastaofsz.
- renameFile on Windows: if os.Rename fails it REMOVES the destination and renames again, which
  leaves a window with no database file at all. On Linux os.Rename is atomic.
- The deferred os.RemoveAll(tmpname) cleans the tmp on the normal path; a crash leaves it. The next
  Shrink's os.Create truncates it, and Open ignores it, so leftovers are already harmless.
- No directory fsync after the rename.
- Writes during Shrink: commits keep appending to the old file; the tail copy under the final lock
  picks them up. Looks correct for concurrent writes (no lost or duplicated commands).

## Upstream tests that touch the on-disk format (buntdb_test.go)
- testFormat block (1370-1440): writes raw legacy RESP files, expects valid/invalid and
  partial-tail handling. A solution that keeps the legacy parser passes these unchanged.
- TestShrink (1655-1715): expects size 0 after shrinking an empty db, and size after shrink smaller
  than before. A new format passes if an empty snapshot writes no bytes.
- TestAutoShrink (1299): size-driven; passes with any compact format.
- No test asserts exact bytes of a file written by the library itself.

## Baseline predictions, written before the run (measured results are in VERIFIERS.md; the one miss was V3, which also fails because the half-applied state from the cut carries through)
| Check | Predicted on unmodified repo | Why |
|---|---|---|
| V1 round trip, rollback, upstream suite | pass | nothing changed |
| V2 truncation sweep | FAIL | a cut inside a multi-command transaction leaves the first commands applied: state is not any S_k |
| V3 append after cut | pass where V2 passes | load() truncates to the last whole command, appends are clean |
| V4 SIGKILL under Always | likely pass | one write() syscall completes even if the process dies right after; the ack comes after Sync |
| V5 last tx bytes altered | FAIL | structural bytes give ErrInvalid instead of discard; payload bytes load a wrong value |
| V6 earlier tx bytes altered | FAIL | payload bytes load silently (file "opens without complaint"); a length digit that overruns EOF truncates later data |
| V7 old fixtures | pass | same code |
| V8 Shrink with concurrent writes, -race | likely pass | tail copy under lock; TO MEASURE |
| V9 SIGKILL during Shrink | likely pass on Linux | rename is atomic; tmp leftovers harmless; may FAIL on Windows (remove-then-rename window) |
| V10 one file, commit cost | pass | append only |
| V11 go.mod unchanged, API unchanged | pass | nothing changed |

So the honest baseline statement is: the unmodified repo must fail V2, V5 and V6, each for the
reason in the table. V9 is a guard on the new Shrink rather than a defect of the old one.
