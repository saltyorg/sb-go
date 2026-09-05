# Stack Context

Generated: 2026-09-04

## Stack
- **Language**: Go 1.27.1 (module `github.com/saltyorg/sb-go`)
- **Framework**: Cobra CLI with Fang; Bubble Tea/Bubbles/Lip Gloss terminal UI; no database layer
- **Build**: GNU Make over the Go toolchain; `make build` is the authoritative local gate
- **Test**: `make test` using Go `testing`, plus the action download shell test
- **Lint**: golangci-lint v2 via `go run ...@latest` (CI gate: yes, through `make build`)
- **Format**: `go fmt ./...` (CI runs it, but does not enforce a clean diff)

## Secondary Languages
- Shell (installer, container/policy tests, embedded benchmark, composite action)
- YAML (GitHub Actions workflows and composite action metadata)
- JSON (Renovate configuration)

## Conventions
- Error handling: explicit returns; wrap operational context with `fmt.Errorf(... %w ...)`; inspect causes with `errors.Is`
- Module structure: thin `main`; `cmd` assembles a fresh Cobra tree; shallow domain packages own host, Git, Ansible, release, terminal, and runtime concerns
- Dependencies: process-scoped dependencies are injected into command construction/context; narrow consumer-side interfaces support tests
- Naming: idiomatic Go MixedCaps; constructors use `New...`; command files follow their CLI command names
- Tests: colocated `*_test.go`, standard `testing`, table/subtests and manual fakes; Linux PTY and integration/security tests where needed

## CI Gates
- `test-gha` (push/PR): `make tidy`, `make test GO_TEST_FLAGS='-exec sudo'`, and `make build` (modernize, format, vet, lint, compile), then live `sb gha`, venv, fact, ownership, and Ansible syntax checks
- `test-install` (installer-path push/PR): policy test plus curl/wget install, runtime verification, and binary repair
- `build_and_release` (main push/tag): Linux amd64 build, embedded-toolchain validation, and development self-update-policy check
- Branch-protection requirements are not declared in the repository, so workflow presence does not prove which jobs are required checks
