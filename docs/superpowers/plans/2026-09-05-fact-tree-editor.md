# Fact Tree Editor Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace the complete legacy `sb fact` command with the approved session-backed Bubble Tea tree editor.

**Architecture:** A concrete `facts.Session` owns catalog, security, retained locks, drift, and persistence. A `factui` package consumes a narrow session interface and owns working state/rendering. `cmd/fact.go` is a no-argument TTY launcher.

**Tech Stack:** Go 1.27, Cobra, Bubble Tea v2, Bubbles v2, Lip Gloss v2, gopkg.in/ini.v1, x/sys/unix.

**Spec:** `docs/superpowers/specs/2026-09-05-fact-tree-editor-design.md`

## Global Constraints

- Replace the whole legacy command; retain no positional/flag compatibility mode.
- Do not create roles or instances or rename existing keys.
- Show stored values in plaintext.
- Retain touched-role locks until editor close and preserve existing file metadata.
- Keep Saltbox's Python writer unchanged.
- Run `make build` as the mandatory final local gate.
- Use Conventional Commits and perform no remote mutation.

---

### Task 1: Session-backed fact store

**Files:** Create `facts/`; remove persistence/locking helpers from `cmd/fact.go`, `cmd/fact_lock.go`, and their old tests only after equivalent store tests pass.

**Interfaces:** Produce concrete `facts.Session` methods `OpenSession`, `Catalog`, `LockRole`, `ReleaseRole`, `Apply`, and `Close`, plus typed catalog, change, drift, and apply-result values defined by the design.

- [ ] Write failing catalog tests for sorted multi-instance data, empty sections, literal values, malformed/unsafe files, and singleton locking; verify RED.
- [ ] Implement safe catalog loading and singleton editor locking; verify GREEN.
- [ ] Write failing tests for cancellable 30-second-compatible role locking, retained locks, drift/reload/release, metadata preservation, every typed mutation, effective diff coalescing, and partial multi-role results; verify RED.
- [ ] Move and deepen the existing lock/atomic-write implementation into `facts.Session`; implement Apply and verify all `facts` tests plus existing `cmd` fact security behavior.
- [ ] Commit as `refactor(facts): extract session-backed fact store`.

### Task 2: Tree editor model and view

**Files:** Create `factui/` with focused model, update, view, and colocated tests. Use the standalone prototype only as behavioral reference.

**Interfaces:** Consume a minimal session interface matching the concrete store. Produce `factui.Run(ctx, session, input, output) error` and a testable Bubble Tea model.

- [ ] Write failing model tests for tree/search navigation, aligned inspector fields, plaintext values, add/edit, immutable keys, staged deletion toggles, disabled descendants, and effective pending review; verify RED.
- [ ] Implement the tree and inspector with responsive dimensions and contextual controls; verify GREEN.
- [ ] Write failing tests for async lock wait/cancel/timeout, drift review/reload/manual retry, unified Save/q/Ctrl+C review, multi-instance role warnings, discard, partial Apply, and lock-count display; verify RED.
- [ ] Implement the asynchronous session commands and all review/error states; verify all `factui` tests.
- [ ] Commit as `feat(fact): add tree editor`.

### Task 3: Replace the Cobra command and qualify it

**Files:** Rewrite `cmd/fact.go`; replace legacy fact command tests; add Linux PTY coverage alongside existing terminal lifecycle tests.

**Interfaces:** `sb fact` accepts no arguments and no local flags, rejects non-TTY execution before opening a session, and runs `factui.Run` with Cobra context/input/output.

- [ ] Write failing command-contract tests proving legacy arguments/flags are gone, non-TTY execution fails clearly, and the TUI runner receives Cobra context and streams; verify RED.
- [ ] Replace the command and delete superseded command-local code/tests; update the command-tree contract hash; verify GREEN.
- [ ] Add a PTY test covering startup/restoration, plaintext edit, staged deletion, Save review, exit review, Apply, Discard, and Ctrl+C behavior.
- [ ] Run focused tests, `make test`, and mandatory `make build`; fix only findings within this feature.
- [ ] Commit as `feat(fact): replace command with tree editor`.

### Task 4: Disposable VM acceptance and final review

**Files:** No additional production files unless acceptance exposes a defect; any fix follows a new RED/GREEN cycle.

- [ ] Prepare one Ubuntu 26.04 `core` VM through the `saltbox_vm` MCP and preserve its `guest_instance_id`.
- [ ] Install the candidate binary and disposable multi-instance fact fixtures, then drive the TUI through a guest PTY to edit/add/delete/apply/discard.
- [ ] Verify exact INI results, UID/GID/mode preservation, multi-instance role warning, and contention/cancellation against a Python-held shared lock.
- [ ] Finish the VM as passed or failed according to evidence and verify final helper state.
- [ ] Run the full local gates again and dispatch whole-branch code review; fix and re-review any important findings.
- [ ] Inspect all local commit subjects for Conventional Commit compliance. Do not push.
