# Announcement File Format Guide

This guide describes the `announcements.yml` format consumed by `sb-go` when running
`update` (and the hidden `announcements` command). It is based on the current
implementation in `internal/announcements`.

## Overview

- Each repo (Saltbox, Sandbox) can ship an `announcements.yml` at the repo root.
- `sb-go` loads the file before and after update, then shows only *new* entries.
- Entries are filtered by optional `required_folder` / `required_file` gates.
- Announcements are rendered as Markdown in the TUI viewer.
- Required migrations can be requested via `migration`.

## File Location

Place the file at the root of the repo:

- `/srv/git/saltbox/announcements.yml`
- `/opt/sandbox/announcements.yml`

(These are the default paths; `sb-go` uses the repo root plus
`announcements.yml`.)

## YAML Structure

Top-level key is `announcements`, a list of items:

```yaml
announcements:
  - date: "2025-01-15"
    title: "Your title here"
    message: |
      Your message here.
```

## Field Reference

Each announcement item supports the following keys:

- `date` (string, required)
  - Expected format: `YYYY-MM-DD` (for correct sorting and nice display).
  - If parsing fails, the raw string is shown.
- `title` (string, required)
  - Used to detect *new* announcements along with `date`.
  - Not currently shown in the UI, but it matters for uniqueness.
- `message` (string, required)
  - Markdown content rendered in the announcement viewer.
  - Use a YAML block scalar (`|` or `>`) for multi-line content.
- `migration` (object, optional)
  - `required` (boolean): if `true` and `tag` is non-empty, user is prompted.
  - `tag` (string): Ansible tag that will be run when approved.
- `required_folder` (string, optional)
  - Announcement is only shown if this path exists *and* is a directory.
- `required_file` (string, optional)
  - Announcement is only shown if this path exists *and* is a regular file.

Notes:

- If both `required_folder` and `required_file` are set, **both must exist**.
- Paths are checked exactly as provided via `os.Stat`, so **absolute paths**
  are recommended to avoid ambiguity.

## How New Announcements Are Detected

`sb-go` compares the *before* and *after* `announcements.yml` files and treats
an entry as "new" if its `date|title` combination does not exist in the before
file. This means:

- Changing only `message` will **not** re-show an announcement.
- Changing `title` or `date` will show it again (as a new entry).
- If `title` is omitted, all items with the same date collide.

Announcements are displayed oldest-to-newest, based on the `date` string.
Use `YYYY-MM-DD` to preserve chronological order.

## Display Behavior

The viewer renders a heading like:

```
# Saltbox Announcement - January 2, 2006
```

Then it renders `message` as Markdown. The terminal renderer uses a word-wrap
width of 78 columns and preserves newlines.

## Migration Behavior

If an announcement includes:

```yaml
migration:
  required: true
  tag: some_tag
```

`sb-go` will:

- Collect all required migrations across new announcements.
- Sort them oldest-to-newest by announcement date.
- Ask once: run all migrations (y/n).
- If approved, run each tag with `ansible-playbook --tags <tag>` against the
  corresponding repo playbook.

If `required` is `true` but `tag` is empty, the migration will be ignored.

## Examples

### Minimal announcement

```yaml
announcements:
  - date: "2025-01-15"
    title: "Settings cleanup"
    message: |
      We removed deprecated settings. Please review your configuration.
```

### With Markdown formatting

```yaml
announcements:
  - date: "2025-02-01"
    title: "New defaults"
    message: |
      ## What changed
      - Default network was updated.
      - Old variables are no longer used.

      ## Action required
      Update your overrides, then rerun `sb update`.
```

### With a migration tag

```yaml
announcements:
  - date: "2025-03-10"
    title: "Database migration"
    message: |
      A database migration is required.
      Approve the migration when prompted.
    migration:
      required: true
      tag: migrate_database
```

### Gated by a file or folder

```yaml
announcements:
  - date: "2025-04-05"
    title: "Only for existing installs"
    message: |
      This applies to systems that already have the legacy file.
    required_file: "/srv/git/saltbox/legacy.conf"

  - date: "2025-04-06"
    title: "Only for hosts with a folder"
    message: "This only appears if the folder exists."
    required_folder: "/opt/some-app"
```

## Common Pitfalls

- **Missing `title`**: you will accidentally merge announcements by date.
- **Non-ISO dates**: sorting and display may be incorrect.
- **Relative paths** in `required_file` / `required_folder`: may not resolve
  as expected depending on how `sb-go` is invoked.
- **Expecting message edits to re-show**: change the title or date if you
  need to re-announce.
