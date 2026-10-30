# Deployment

## Native Linux / systemd / LXC

Build on a machine with Node 24+, npm, Go 1.26+, and Make, or use the archives produced by `make release`. The target machine runs only the compiled executable. SQLite is compiled into it; there is no system SQLite runtime dependency.

Native archives contain the executable, `config.example.yaml`, the systemd unit, and documentation. When installing from an archive, use those files in place of the source-tree paths in the example below.

An example installation using systemd:

```sh
sudo useradd --system --home /var/lib/opnsense-ip-invites --shell /usr/sbin/nologin ip-invites
sudo install -m 0755 dist/opnsense-ip-invites /usr/local/bin/opnsense-ip-invites
sudo install -d -m 0750 -o root -g ip-invites /etc/ip-invites
sudo install -m 0640 -o root -g ip-invites examples/config.yaml /etc/ip-invites/config.yaml
sudo install -m 0644 deploy/opnsense-ip-invites.service /etc/systemd/system/
```

Edit `/etc/ip-invites/config.yaml`, including:

```yaml
database: "/var/lib/opnsense-ip-invites/invites.db"
```

Place the three required secret environment variables in `/etc/ip-invites/secrets.env`, owned by root with mode `0600`. EnvironmentFile syntax is `NAME=value`; shell commands are not executed. Alternatively, point the variables ending in `_FILE` at files readable by `ip-invites`. Keep a custom CA certificate under `/etc/ip-invites` so the service's ProtectHome restriction does not hide it.

```sh
sudo /usr/local/bin/opnsense-ip-invites -config /etc/ip-invites/config.yaml -check-config
sudo systemctl daemon-reload
sudo systemctl enable --now opnsense-ip-invites
sudo journalctl -u opnsense-ip-invites -f
```

systemd creates the writable state directory with the appropriate ownership. Keep the backend restricted to your proxy, configure its admin authentication and visitor headers, and expose HTTPS at the configured public URL. There is no LXC- or hypervisor-specific runtime integration.

`-check-config` validates YAML only. It does not contact OPNsense, create a database, or check credentials. Running normally starts the server and reconciliation worker; missing firewall connectivity appears as a readiness failure and an admin sync error.

## Docker

```sh
cp examples/config.yaml config.yaml
```

Edit the configuration for a container:

```yaml
server:
  listen: "0.0.0.0:8080"
  # Set the other server fields, including actual trusted proxy CIDRs.
database: "/data/invites.db"
```

Create a local `.env` with `OPNSENSE_API_KEY`, `OPNSENSE_API_SECRET`, and `INVITES_ADMIN_PROXY_SECRET`, then:

```sh
docker compose --env-file .env -f deploy/compose.yaml up --build -d
```

The example publishes only host loopback port 8080. Adjust networking for your proxy deliberately rather than opening the backend directly to the internet. Mounted configuration is read-only and data uses a named volume. The process runs as UID/GID **65532** without Linux capabilities. Bind-mounted data directories must be writable by that user. Custom CA files also need a read-only mount and matching `opnsense.ca_file`.

The Dockerfile supports BuildKit's target architecture arguments:

```sh
docker build -t opnsense-ip-invites:local .
docker buildx build --platform linux/amd64,linux/arm64 --output type=oci,dest=dist/image.tar .
```

Nothing is pushed or published by these commands. A container must retain its `/data` volume across replacement. Runtime has no shell; use external HTTP probes instead of expecting `docker exec ... sh`.

## Health, outages, and backups

- `/healthz`: process liveness.
- `/readyz`: 200 only when all registered alias synchronizations are confirmed; otherwise 503.
- Admin list cards show desired/actual counts, last sync state, and errors. **Sync now** retries immediately.
- On API failure, desired changes stay in SQLite. Authorization responds as pending, and revocation returns HTTP 202 when recorded but not yet confirmed remotely.
- If the app stops, existing alias entries remain until something changes them or the firewall restarts. Expiry is application-managed, not a firewall-enforced deadline.
- Revocations remove membership after successful synchronization; existing stateful connections may survive.

For a simple consistent backup, stop the single application instance, copy its database, and restart it. Alternatively use a proper SQLite online backup tool; copying only the main file while WAL writes are active is not a consistent backup. Store backups with restrictive permissions. Restore the database together with the intended configuration, then inspect synchronization before returning the service to use.

Do not run multiple replicas or place the database on an unreliable/network filesystem. The app uses a process lock beside the database and will reject another local instance. Back up before upgrading; a database from a newer schema version is rejected by older binaries. V1 retains grant history; apply your own retention/backup policy and do not reset the database without considering memberships left in OPNsense.
