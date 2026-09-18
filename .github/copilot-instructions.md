# Copilot Coding Agent Instructions for justimport

## Project Overview

**justimport** is a Go CLI tool that automatically resolves "Manual Import Required" queue items in Radarr and Sonarr. It polls the Radarr/Sonarr APIs, identifies stuck queue items, and auto-imports single-file matches that pass safety checks. It has **zero external dependencies** — only the Go standard library is used.

## Repository Layout

```
cmd/justimport/main.go       — Application entry point (CLI, config loading, signal handling)
internal/arrclient/           — HTTP client for Radarr/Sonarr v3 API
  client.go                   — Client implementation (GET/POST helpers, queue, manual import)
  client_test.go              — Unit tests using httptest
  types.go                    — Data structures for API request/response payloads
internal/config/              — Configuration from environment variables
  config.go                   — Loader and validation
  config_test.go              — Unit tests
internal/importer/            — Core import logic (polling, filtering, importing)
  importer.go                 — Main business logic
  importer_test.go            — Unit tests with mock client
Makefile                      — Build, test, lint, gopls and check targets
Dockerfile                    — Multi-stage build (golang:1.27-alpine → distroless)
docker-compose.yml            — Example Docker Compose configuration
.golangci.yml                 — golangci-lint v2 configuration with ~20 linters enabled
.github/workflows/test.yml    — CI: lint, unit tests, build (runs on push to main and PRs)
.github/workflows/release.yml — CD: Docker image build and publish to GHCR on release
```

## Build, Test, and Lint Commands

```bash
# Build the binary (output: ./justimport)
make build

# Run all tests with race detector
make test

# Run linter (requires golangci-lint installed)
make lint

# Run static analysis with gopls (requires gopls installed)
make gopls

# Run tests, lint and gopls together
make check

# Run the application (requires env vars, see Configuration below)
make run
```

### Notes on Local Development

- **golangci-lint** and **gopls** are not installed by default in this sandbox. CI installs golangci-lint via the `golangci/golangci-lint-action`. To install locally: `go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@latest` and `go install golang.org/x/tools/gopls@latest`, then put `$(go env GOPATH)/bin` on your `PATH`. If installation is not feasible, rely on `go vet ./...` for basic static analysis and let CI run the full lint suite.
- All tests use the standard `testing` package. No test framework is needed.
- Tests use `httptest.NewServer` for HTTP mocking and mock implementations of the `ArrClient` interface for the importer.
- The build produces a statically linked binary (`CGO_ENABLED=0`).

## Configuration

The application is configured **entirely via environment variables** (no config files):

| Variable | Default | Description |
|---|---|---|
| `RADARR_URL` / `RADARR_URL_<N>` | *(unset)* | Base URL of each Radarr instance |
| `RADARR_API_KEY` / `RADARR_API_KEY_<N>` | *(unset)* | Matching Radarr API key |
| `SONARR_URL` / `SONARR_URL_<N>` | *(unset)* | Base URL of each Sonarr instance |
| `SONARR_API_KEY` / `SONARR_API_KEY_<N>` | *(unset)* | Matching Sonarr API key |
| `POLL_INTERVAL` | `60s` | Go duration for queue poll interval |
| `DRY_RUN` | `true` | Set to `false` to enable actual imports |

Add instances by appending an incrementing suffix — `RADARR_URL_1` / `RADARR_API_KEY_1`, then `_2`, and so on. The numbers must start at `1` and be contiguous: the loader stops at the first unset number, so `RADARR_URL_3` is ignored when `RADARR_URL_2` is missing. At least one Radarr or Sonarr URL must be set, otherwise `Load()` returns an error. An `API_KEY` whose `URL` is unset is ignored; a `URL` whose `API_KEY` is unset loads and polls with an empty key.

`Load()` returns `Config.Instances`, a `[]InstanceConfig` of `Type` / `URL` / `APIKey`, ordered base Radarr, numbered Radarr, base Sonarr, numbered Sonarr. `main.go` builds one `arrclient.Client` per entry.

## Coding Conventions

- **Go version**: 1.27 (specified in `go.mod`). Use only standard library packages.
- **Zero dependencies**: Do not add external dependencies. The project intentionally uses only the Go standard library.
- **Module path**: `github.com/erkexzcx/justimport`
- **Package structure**: All internal packages live under `internal/`. The only public entry point is `cmd/justimport/main.go`.
- **Error handling**: Wrap errors with `fmt.Errorf("context: %w", err)`. Return errors rather than panicking.
- **Logging**: Use `log/slog` (structured logging). The application uses a custom `slog.Handler` for human-readable output. Always sanitize external input in log messages using the `sanitizeLog()` helper to prevent log injection.
- **Naming**: Follow standard Go conventions — exported names are PascalCase, unexported are camelCase. Interface names describe behavior (e.g., `ArrClient`).
- **Testing**: Use the standard `testing` package with descriptive test function names (`TestLoad_DryRunFalse`, `TestProcessItem_ZeroFiles`). Use `t.Setenv()` for environment variable tests. Tests that need HTTP call mocking should use `httptest.NewServer`.
- **Linting**: The `.golangci.yml` enables strict linters including `errcheck`, `govet`, `staticcheck`, `gosec`, `gocritic`, `revive`, `errorlint`, and others. Ensure all errors are checked, type assertions are validated, and `nolint` directives include a reason comment.
- **nolint directives**: When suppressing a lint warning, use `//nolint:lintername // reason` format (see examples in `client.go`).
- **Security**: API response bodies are limited to 10 MB (`maxResponseBytes`). HTTP clients use a 30-second timeout. Log messages sanitize external input (newlines replaced). `DRY_RUN` defaults to `true` for safe-by-default behavior.

## CI Pipeline

The `test.yml` workflow runs three parallel jobs on every push to `main` and on PRs:
1. **Go Lint** — `golangci-lint` with 5-minute timeout
2. **Go Unit Tests** — `go test -v -race -coverprofile=coverage.out ./...`
3. **Build Binary** — `make build`

All three jobs must pass for a PR to merge.

## Key Design Patterns

- **Interface-based testing**: The `importer` package defines `ArrClient` as an interface, allowing tests to use mock implementations without external dependencies.
- **Instance identity vs. API behaviour**: `arrclient.Client` keeps `appType` (`"radarr"` / `"sonarr"`, which selects API behaviour such as the queue query parameter) separate from `label`. `Name()` returns the label, e.g. `Radarr (http://radarr4k:7878)`, so several instances of the same app stay distinguishable. The importer keys its `lastFailed` map on `Name()`, so the label must remain unique per instance.
- **Poll loop with graceful shutdown**: The `Importer.Run()` method uses `context.Context` cancellation with `signal.NotifyContext` for clean shutdown on SIGINT/SIGTERM.
- **Deduplication**: Processed download IDs are tracked in a `seen` map to avoid re-processing items on subsequent polls. The map is keyed by download ID alone and is shared across all clients, so the same download ID appearing in two instances' queues counts as one item.
- **Safety-first filtering**: Items with multiple files, rejections, or no match are skipped. Sample files (path containing "sample", case-insensitive) are filtered out before evaluation.
