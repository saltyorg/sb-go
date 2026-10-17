# Development workflow

When changing Go code or its build workflow, complete these steps before
committing or handing work back:

1. Run `make modernize`. Review the resulting diff for correctness and relevance
   to the task before accepting the generated changes.
2. Run `make fmt` and review the resulting diff.
3. Run the relevant tests and `make build`. Repeat affected validation after any
   further changes, and inspect the final diff before committing.

Keep `make build` and `make check` free of source rewrites. Modernization and
formatting are explicit preparation steps; `make fmt-check` checks formatting
without changing files.
