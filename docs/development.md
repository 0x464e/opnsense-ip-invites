# Development

The Go module is `github.com/0x464e/opnsense-ip-invites`, matching the intended public repository. Build commands work from any checkout path and do not require a configured Git remote. The frontend starts from Vite's React/TypeScript template. Dependencies are locked by `go.sum` and `web/package-lock.json`.

Requirements: Node 24+, npm, Go 1.26+ with the latest patch, and Make. The Go race detector also needs a C compiler (such as GCC), although production binaries use CGO_ENABLED=0. `.node-version` records the Node major. No GitHub pipelines are installed in v1.

## Commands

```sh
make build                  # npm ci, TS check, frontend bundle, embedded native binary
make test                   # backend race tests and TS check
make test-browser           # desktop/mobile end-to-end tests
make release VERSION=v0.1.0 # Linux amd64/arm64 archives and checksums
make check-config           # validate the example without side effects
python3 scripts/smoke-test.py # exercise a built production binary against a mock firewall
```

If a host C compiler is unavailable, `docker build --target test .` runs the backend race suite in the Go build image instead. This does not publish an image or contact a live firewall.

Install the browser once:

```sh
cd web
npm ci
npx playwright install chromium
```

Playwright may require operating-system browser libraries. Use its documented dependency installation for your development machine if it reports missing libraries. Browser tests start a Go fixture and Vite on loopback ports 18080 and 5173, then stop them. They use real application handlers, SQLite, and the real OPNsense HTTP client connected to a local mock firewall. The fixture's test-only address/admin-header injection is excluded from normal builds by the `e2e` build tag.

## Interactive mock preview

This is a local development fixture, not a production deployment:

```sh
# Repository root, first terminal:
go run -tags e2e ./internal/testsupport/e2e

# Second terminal:
cd web
DEV_API_URL=http://127.0.0.1:18080 npm run dev
```

Open `http://127.0.0.1:5173/admin`. To find a seeded visitor link, inspect `http://127.0.0.1:18080/__fixture/invite`. The fixture pretends to be an authenticated proxy and supplies a synthetic test visitor address; it never reaches a real firewall. Its temporary database is removed on graceful exit. Stop both processes when finished.

## Develop against your own test environment

1. Build or run the backend with a separate test YAML and dedicated, disposable OPNsense aliases.
2. Set the YAML public URL to `http://127.0.0.1:5173` for local Vite development.
3. Trust only loopback for the local API connection. Set `client_ip_header` appropriately for your test proxy; loopback itself cannot be authorized as a public IPv4.
4. Start Vite with `DEV_API_URL` and `DEV_ADMIN_PROXY_SECRET` set in its server environment. Use the same proxy secret as the backend.

`DEV_ADMIN_PROXY_SECRET` is consumed by Vite's Node-side proxy, never injected into the frontend. Do not prefix secrets with `VITE_`, expose the dev server publicly, or use the browser fixture with real deployment credentials.

A normal `go run ./cmd/opnsense-ip-invites` serves a development notice for HTML; Vite serves the actual UI. Production builds use `-tags production` after the frontend has been built and embed `web/dist`. `make build` handles the order. Go tests do not require generated frontend files.

## Architecture

- `internal/config`: strict YAML validation, required defaults, path resolution, and environment/file secrets.
- `internal/store`: schema migration, tokens, transactional grant updates, history, and authoritative membership queries.
- `internal/firewall`: authenticated, verified HTTPS API, full pagination, and canonical address parsing.
- `internal/service`: serialized grants/reconciliation, alias validation, restart restoration, and sync status.
- `internal/httpapi`: proxy trust, admin boundary, Origin checks, rate limits, JSON API, and frontend serving.
- `web/src`: React visitor/admin routes, dialogs, and API queries.

Tests cover overlapping invitations and revocation, natural link expiry, renewals and per-link limits, concurrency, persisted recovery, retired alias cleanup, API acknowledgements without actual mutation, unsafe aliases, spoofed proxy headers, IPv6 rejection, admin protection, TLS verification, and complete browser workflows.
