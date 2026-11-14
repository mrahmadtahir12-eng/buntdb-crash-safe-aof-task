# Where the expertise comes from, the hard part, and where models go wrong

## Where it comes from
I have spent time on storage and recovery bugs, the kind that only show up when a process dies
halfway through a write. Models write the happy path of an append-only log very well. They are
much weaker on crash semantics, because the interesting states never appear in a normal run.
The problem is also real for BuntDB's users: issue #82 upstream (open since January 2022) reports
corrupted databases after partial writes, the maintainer preferred fixing the root cause over a
repair tool, and nothing has landed beyond error wrapping.

## What the pinned code actually does (verified by reading buntdb.go at 417eb7c)
- The file is a Redis-style AOF: one RESP command per SET, DEL or FLUSHDB. There is no
  transaction boundary on disk. A commit with k changes is k commands written with a single
  `file.Write`.
- On Open, a command that runs into end-of-file is treated as a torn tail and the file is truncated
  back to the last whole command. That is command granularity, so a transaction cut in the middle
  is replayed half applied: a multi-key update half applied after a reboot.
- Any structural fault (a missing `*`, a bad count, a payload not followed by CRLF) returns
  `ErrInvalid`. But an altered byte inside a key or value loads silently with the wrong contents,
  and an altered length digit that makes a command overrun the end of the file is mistaken for a
  torn tail, so Open truncates everything from that point and opens cleanly: a database that opens without complaint but is missing recent writes.
- Shrink writes a temp file, copies the commands committed since it started, fsyncs and renames.
  The rename is atomic on Linux, leftover temp files are already harmless, and concurrent writes are
  picked up by the tail copy. The Shrink requirement mostly guards the model's new design, not an
  existing defect.

## The hard part in two sentences
A transaction must become atomic inside an append-only file that old files also live in, and the
reader must tell a torn tail (normal after a crash, safe to drop) from corruption in the middle
(must be reported, never skipped). Both look like "the bytes stop making sense", and the trap is
that a corrupted length field makes a middle record look exactly like a torn tail.

## What the model must discover on its own
- How the current loader treats a partial command, and that its choice decides which of those two
  failures can happen.
- That the per-command loader applies half of a multi-command commit.
- That old files contain no framing at all, so the new reader must accept a file that starts with
  old records and continues with new ones.
- That an interrupted Shrink leaves files behind, and that the swap has to be safe at every step.

## Where I expect models to go wrong
- Checksums per command instead of per transaction, so half a transaction is still applied.
- Treating every undecodable byte as a torn tail, which silently drops later data (the silent
  loss described above).
- A length field without its own protection, so one flipped digit in the middle turns into a
  "torn tail" and the rest of the file is thrown away.
- Truncating a torn tail but not the garbage after it, so later commits are appended behind bytes
  the reader refuses to pass.
- A new file header or framing that old files cannot open under, or a file that breaks once old
  and new records are mixed.
- Resynchronising by scanning for a magic byte, which a value that imitates framing bytes fools.
- Shrink that loses writes made while it runs, or that swaps files without considering a crash
  between the steps, or whose rewritten tail no longer chains to the snapshot.
- "Fixing" durability by rewriting the whole file on every commit, which the cost constraint
  forbids.
