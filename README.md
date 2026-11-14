# Task: crash-safe persistence for BuntDB

A long-horizon backend task for an assessment. The model is handed a checkout of
[tidwall/buntdb](https://github.com/tidwall/buntdb) (Go, MIT) and asked to make its append-only
persistence crash-safe: atomic transactions on disk, a torn tail that is discarded, corruption in
the middle that is reported, old files that still open, and a Shrink that survives being killed.

Upstream pin: commit
[417eb7c4c8b654adf2b56784d18c6a80136d445a](https://github.com/tidwall/buntdb/commit/417eb7c4c8b654adf2b56784d18c6a80136d445a)
(2026-11-07). All the work touches `buntdb.go`: `load`/`readLoad`, `Tx.Commit`, `Shrink`,
`backgroundManager`. The problem is a real, unsolved one upstream: see
[tidwall/buntdb#82](https://github.com/tidwall/buntdb/issues/82), open since January 2022, where corrupted
databases after partial writes were reported and only an error-wrapping change landed. This repository
publishes the write-up only; BuntDB is not forked or changed here.

| File | What |
|---|---|
| [TASK_PROMPT.md](TASK_PROMPT.md) | the prompt, exactly as the model receives it, nothing else |
| [EXPERTISE.md](EXPERTISE.md) | where the idea comes from, the hard part, where models go wrong |
| [VERIFIERS.md](VERIFIERS.md) | one row per check, the edge cases, how gaming is resisted, the validation plan and its status |
| [verifier/](verifier/) | the Go harness for every check: `run.sh <candidate dir>` builds a clean tree and runs every check (V1a to V11b); `gen_fixtures.sh` regenerates the old-format fixtures from the pinned upstream code |
| [notes/CODE_STUDY.md](notes/CODE_STUDY.md) | what the pinned persistence code does, with line numbers |
| [notes/baseline_quick.log](notes/baseline_quick.log), [notes/reference_quick.log](notes/reference_quick.log) | the quick-mode runs on the unmodified checkout and on a reference solution (the reference itself is not published, since it is the answer key) |
