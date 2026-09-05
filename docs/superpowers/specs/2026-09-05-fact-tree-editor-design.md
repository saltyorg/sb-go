# Fact Tree Editor Design

## Goal

Replace the complete legacy `sb fact [role] [instance] --method=...`
interface with the approved full-screen Bubble Tea tree editor. The command
accepts no positional arguments or legacy flags, requires an interactive TTY,
and edits existing Saltbox fact files without masking values.

## Persistence and locking

Create a concrete `facts.Session` module. It safely catalogs existing `*.ini`
role files, owns a singleton `.fact-editor.lock`, and retains each
`<role>.ini.lock` from the first mutation until the editor closes. Browsing does
not lock role files. Lock acquisition waits up to 30 seconds and is cancellable.

After acquiring a role lock, re-read its bytes and compare them to the catalog
fingerprint. Drift opens a semantic review. Reloading adopts current disk state,
retains the lock, cancels the attempted mutation, and requires a manual retry.
Cancelling releases the newly acquired role lock.

Apply supports setting or adding a fact in an existing instance, deleting a
fact, deleting an instance, and deleting a role. It cannot create roles or
instances or rename keys. Preserve the existing UID, GID, and mode along with
the current symlink, regular-file, same-inode, same-directory temporary file,
file/directory fsync, and atomic-rename guarantees. Saltbox's Python writer
already follows the same role-lock protocol and is unchanged.

Multi-role Apply preflights and fsyncs all replacements, then commits in sorted
role order. On failure, report applied, failed, and unattempted roles; retain
failed and unattempted changes and do not attempt rollback.

## Terminal interaction

Use the accepted expandable role -> instance -> key tree with a contextual
inspector. Tree rows contain hierarchy and keys; the inspector displays aligned
`Role:`, `Instance:`, `Key:`, and `Status:` fields and plaintext values. Sort
roles, instances, and keys for navigation while preserving underlying INI data.

`/` filters with matching ancestors retained. Arrows and Enter navigate and
expand/collapse. `a` adds a key/value only to an existing instance. `e` edits a
value. `d` toggles a staged deletion immediately. Marked nodes remain visible
and dimmed, and marked parents disable descendant mutation.

`s` opens Review Changes. `q` and Ctrl+C open the same review when changes are
pending. Review offers Apply, Discard, or Return; exit-triggered review offers
Apply-and-exit or Discard-and-exit. A role deletion affecting more than one
instance lists every affected instance. SIGTERM writes nothing and releases all
locks.

## Failure behavior

Reject non-TTY invocation before opening locks. Fail startup on a missing or
unsafe facts directory or any unreadable, unsafe, or malformed role file. If
another TUI owns `.fact-editor.lock`, report that another editor is active.
Keep pending changes after lock, validation, or write errors. Apply-and-exit
remains open after partial failure.

## Acceptance

Use TDD for the store and TUI. Cover real temporary-file security and metadata,
session/role locks, drift, partial apply, all model transitions, command
contract removal, and Linux PTY lifecycle. Run `make test`, mandatory
`make build`, and Ubuntu 26.04 core-VM acceptance against disposable fixtures.
