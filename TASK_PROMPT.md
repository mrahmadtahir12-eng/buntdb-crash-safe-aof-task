I need the persistence in buntdb.go (tidwall/buntdb, the checkout in this repo) made crash-safe without breaking the public API or anyone's existing data files.

What it has to do:

- Commits are all-or-nothing on disk. If the process dies, or the file ends, at any byte while a transaction is being written, reopening shows the database as it was after some whole committed transaction, never part of one. This holds for every SyncPolicy.
- With SyncPolicy Always, a transaction whose Update returned nil is still there after the process is killed right afterwards. A rolled-back transaction leaves no trace.
- If any byte of the last transaction is altered, or the file is cut anywhere inside it, Open discards that transaction and leaves the file so that later commits append cleanly and survive another reopen.
- If bytes are altered (file length unchanged) inside any earlier transaction, Open must not silently skip it or what follows: it returns an error for which errors.Is(err, buntdb.ErrInvalid) is true, and leaves the file unchanged.
- Intact data files written by the current version still open and replay to the same contents, and keep working after the new code appends to them, so one file may hold old and new data.
- Shrink running alongside writes loses and duplicates nothing committed before, during or after it. If the process is killed at any moment during Shrink, including while it swaps files, the next Open sees the correct contents, and leftover files from that Shrink don't stop the next Open or Shrink.
- The database remains one file at the path given to Open (apart from short-lived files during Shrink), and the cost of a commit doesn't grow with the size of the database.

Keep the exported API and in-memory behaviour (indexes, iteration, TTLs, one writer at a time) as they are, and add no dependencies beyond the standard library and what go.mod already has.
