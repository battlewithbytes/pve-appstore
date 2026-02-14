# Repository Guidelines

## Project Structure & Module Organization
`cmd/pve-appstore/` contains the CLI entry point. Backend code lives in `internal/` with major modules like `config/`, `catalog/`, `server/`, `engine/`, `proxmox/`, and `installer/`. The Python provisioning SDK is in `sdk/python/appstore/`. The React + TypeScript frontend is in `web/frontend/`. Test fixtures live under `testdata/` (notably `testdata/catalog/`). Deployment assets are in `deploy/`, and docs/screenshots are in `docs/`.

## Build, Test, and Development Commands
Use the Makefile targets for day-to-day work:
- `make deps` installs Go modules and frontend npm packages.
- `make build` compiles the `pve-appstore` binary with version metadata.
- `make test` runs Go tests (SDK tests are in `sdk/python`).
- `make frontend` builds the React SPA in `web/frontend`.
- `make run-serve` runs the dev server with `dev-config.yml` and `testdata/catalog`.
- `make test-apps` validates app manifests against the test catalog.
- `make release` cross-compiles Linux amd64/arm64 binaries.

## Coding Style & Naming Conventions
Go code is formatted with `gofmt` (`make fmt`) and vetted with `go vet` (`make vet`). Indentation follows standard Go and TypeScript defaults (tabs in Go, 2-space in TS). Prefer descriptive, domain-specific names aligned with existing packages (e.g., `catalog`, `engine`, `proxmox`). Frontend builds are driven by npm scripts in `web/frontend`.

## Testing Guidelines
Go tests run via `go test ./...` (`make test`). Python SDK tests live under `sdk/python` (run with `python3 -m pytest tests/` from that directory). Keep test data under `testdata/`, and name tests using Go’s `TestXxx` convention.

## Commit & Pull Request Guidelines
Recent commits use short, imperative summaries like “Add …”, “Fix …”, or “Update …” without scopes. Match that style unless the team requests otherwise. For pull requests, include a concise summary, list user-facing changes, and add screenshots for UI changes (see `docs/screenshots/` conventions). Link related issues if applicable.

## Security & Configuration Notes
This project interacts with Proxmox and uses API tokens; follow `SECURITY.md` for the security model. Development runs typically use `dev-config.yml`; keep credentials out of git and avoid logging secrets.
