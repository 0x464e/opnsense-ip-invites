# opnsense-ip-invites

Self-service IPv4 invitations for OPNsense aliases.

Create a link, share it, and let its visitors authorize their current public IPv4 address. An admin chooses the target list, link expiry, access duration, and optional IP limit. The application keeps the corresponding OPNsense alias synchronized as grants are created, renewed, revoked, and expired.

This is an independent project for OPNsense users. It manages alias membership; you decide how those aliases are used. It creates missing External aliases but does not create or apply firewall rules or port forwards.

## How it works

1. Open `/admin` through your authenticated reverse proxy and create a list. Its missing OPNsense External alias is created automatically.
2. Reference that alias in an enabled firewall policy and apply the changes in OPNsense.
3. Create an invitation for the list in `/admin`.
4. A visitor opens the link on the computer and connection they intend to use, then clicks **Authorize me**.
5. The application detects their public IPv4, records a grant, updates OPNsense, and confirms membership before displaying success.

An alias contains the union of all active grants for its list. If Friday's invitation grants an IP access until Saturday, and Saturday's invitation grants the same IP access until Sunday, Friday's grant expiring **does not remove the IP**. Membership ends only when the last grant requiring it is expired or revoked.

Link validity and access duration are independent. A naturally expired link cannot authorize again, but grants already issued through it retain their expiry. Admin revocation can disable only the link or also revoke its grants.

## Features

- Minimal React/TypeScript visitor page and admin panel.
- Create, edit, and delete lists in the admin panel, each backed by a dedicated OPNsense External alias.
- Reusable labeled links for individuals or groups, with defaults supplied by YAML.
- Optional IP limits: `0` is unlimited; `1` replaces that invitation's previous IP; higher limits reject new IPs when full.
- Independent, persistent grants and effective expiry across overlapping invitations.
- Automatic reconciliation, outage recovery, and restoration after firewall restarts.
- Reverse-proxy admin authentication, explicit trusted proxies, and verified HTTPS to OPNsense.
- A native Linux executable with the frontend embedded, plus a non-root Docker image.

## Build

Requirements: **Node.js 24.15+, npm, Go 1.26+ (latest patch recommended), and Make**. Go's automatic toolchain selection can install the version requested by `go.mod`.

```sh
make build
./dist/opnsense-ip-invites -version
```

The executable contains the production frontend. The deployment machine does not need Node, Go, Docker, or a separate database server. Native targets currently supported are Linux amd64 and arm64, including suitable LXC containers.

```sh
make test                   # Backend race tests and frontend type checking
cd web && npx playwright install chromium && cd ..
make test-browser           # Desktop and mobile browser tests against a mock firewall
make release VERSION=v0.1.0 # Native archives and SHA-256 checksums; publishes nothing
```

Tests use local mocks and do not require OPNsense credentials. See [development](docs/development.md).

## Configure and run

```sh
cp examples/config.yaml config.yaml
# Edit config.yaml and configure the proxy and OPNsense as described below.
./dist/opnsense-ip-invites -config config.yaml -check-config

# Supply these through your service manager, shell environment, or *_FILE secrets.
export OPNSENSE_API_KEY='your-dedicated-api-key'
export OPNSENSE_API_SECRET='your-dedicated-api-secret'
export INVITES_ADMIN_PROXY_SECRET='a-random-secret-of-at-least-32-characters'

./dist/opnsense-ip-invites -config config.yaml
```

Generate a real admin proxy secret, for example with `openssl rand -hex 32`; use the same value in the app and authenticated proxy route. The example secret above is explanatory, not a value to deploy.

Each secret also supports a file: `OPNSENSE_API_KEY_FILE`, `OPNSENSE_API_SECRET_FILE`, and `INVITES_ADMIN_PROXY_SECRET_FILE`. Set either the direct variable or its `_FILE` counterpart, not both. Secret values never go into frontend build variables.

The YAML file supplies `defaults.link_validity`, `defaults.access_duration`, and `defaults.max_ips`; the admin form uses those values and allows per-invitation overrides. Durations use Go syntax, such as `30m`, `8h`, or `168h`. Relative database and CA paths resolve against the YAML file's directory. Configuration changes require a restart.

Read these before exposing the application:

- [OPNsense integration and grant lifecycle](docs/opnsense.md)
- [Reverse proxies, authentication, and Cloudflare](docs/reverse-proxy.md)
- [Native/systemd and Docker deployment](docs/deployment.md)
- [HTTP API](docs/api.md)

## IPv4 only

V1 authorizes **public IPv4 addresses only**. A visitor arriving over IPv6 cannot be authorized from that request. The page explains the mismatch and can offer an optional `server.ipv4_public_url` pointing to the same app through an actually IPv4-only public HTTPS endpoint.

Cloudflare Tunnel can accept IPv6 visitors even when the entire origin network is IPv4-only. An IPv4 origin, a second ordinary proxied hostname, or Cloudflare's **Pseudo IPv4** does not provide the visitor's real IPv4. Deployment must supply a genuine IPv4 observation path when this is needed. See [the proxy guide](docs/reverse-proxy.md#ipv4-only-entry).

## Operational boundaries

Run one application instance with persistent local SQLite storage and exclusively app-owned aliases. Do not manually maintain addresses in those aliases: synchronization removes entries not required by the database. Retired aliases remain registered in the database for cleanup after list deletion.

Expiry cleanup depends on the app being running and the firewall API being reachable. Removing membership does not necessarily terminate existing stateful connections. IP-based authorization identifies an internet connection, not a person; household members and carrier-NAT subscribers can share an address. A browser VPN or different route from the protected application may also produce a different source address.

Treat invitation links as credentials. New URLs are shown once because only token hashes are stored. If a link is lost, create a replacement and revoke the old one as appropriate. Retain the database securely; it contains invitation metadata and IP history. See [security](SECURITY.md).

No hosted services or telemetry are required. GitHub CI validates changes on `master` and pull requests. Releases use a manually triggered release-it + GoReleaser workflow, with Conventional Commits, native archives, checksums, and multi-architecture Docker images. See [release setup and operation](docs/releases.md).

## License

[MIT](LICENSE). OPNsense is a trademark of its respective owner; this application is not an official OPNsense product.
