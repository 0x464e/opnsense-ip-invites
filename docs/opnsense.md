# OPNsense integration

## Prepare dedicated aliases

In **Firewall → Aliases**, create and apply an alias of type **External (advanced)** for each configured list. For the example configuration, its name is `INVITED_CLIENTS`. Use aliases that are dedicated exclusively to this application and initially empty. Keep the OPNsense alias enabled.

Use the alias as a source or destination in your own rules. The application has no knowledge of what those rules permit or deny. Membership management is generic; the invitation page's display name and description should accurately explain the access your own policies provide.

An alias must also exist in the active PF tables for membership updates to work. If updates are acknowledged but verification fails, ensure the alias is applied and referenced by an enabled policy. Inspect **Firewall → Diagnostics → Aliases**.

Do not reuse an existing shared or manually maintained alias. The application's desired membership is authoritative, and reconciliation deletes extra entries from every alias it has registered.

## API account

Create a dedicated local API user and generate an API key/secret. On current OPNsense installations the table-operation privilege is **Diagnostics: PF Table IP addresses**. Also grant read access to alias definitions, normally through **Firewall: Aliases**, so the application can verify the type and enabled state. Names and granularity may vary by OPNsense version; inspect the user's Effective Privileges.

Required endpoints:

| Operation | Endpoint |
|---|---|
| Read alias definitions | `GET /api/firewall/alias/search_item` |
| Read current table membership | `GET /api/firewall/alias_util/list/{alias}` |
| Add an address | `POST /api/firewall/alias_util/add/{alias}` |
| Remove an address | `POST /api/firewall/alias_util/delete/{alias}` |

Mutation bodies are `{"address":"203.0.113.42"}`. The API uses Basic authentication with the API key and secret. The application handles pagination and verifies final membership rather than relying on `status: done` alone.

These privileges may permit operations on tables beyond the configured aliases. The application constrains its own operations, but the API credential itself is not an alias-scoped credential. Restrict network access to the firewall management interface and keep the credential on the server.

Use a trusted certificate or configure `opnsense.ca_file` with a PEM CA bundle. TLS verification cannot be disabled. HTTP is accepted only for loopback mock/development endpoints.

The implementation follows the current `alias` and `alias_util` API contracts. Automated tests use a mock rather than claim validation against every OPNsense release.

## Grants and expiry

A grant belongs to one invitation and one IP. It expires independently of the invitation link. Reauthorizing the same IP through the same link renews that grant; another invitation produces a separate grant.

For a list, membership is the union of IPs appearing in any active grant. The admin grant table shows both the individual grant expiry and the latest effective expiry for that IP in the same list. An expired or revoked grant row can still show effective access when another active grant remains.

For a one-IP invitation, authorization from a new IP revokes that link's previous grants. Grants through other links remain intact. Group limits count distinct currently active IPs for that link. A renewal from an already active IP is allowed even at capacity.

Changes and reconciliation are serialized in a single application instance. Additions precede removals during reconciliation, and final membership is checked before successful authorization is reported. A failed update leaves desired membership recorded and returns a pending/error response; periodic reconciliation retries it. A pending request still consumes that invitation's IP slot and can be retried from the same address.

The sync interval determines routine expiry cleanup latency. API outages, app downtime, and slow API responses can extend that latency. Stopping the app does not empty the firewall tables. OPNsense restarts normally clear External table content, and the next successful reconciliation restores still-active grants.

## Configuration changes

On startup, aliases in the configuration are added to a persistent managed-alias registry. Aliases previously registered but no longer referenced are reconciled to an empty table. If a retired alias has already been deleted from OPNsense, cleanup considers it absent.

Changing a list ID creates a logically different list; its old invitation links stop authorizing. Changing the alias for an existing list moves its desired membership to the new alias. Old aliases are cleaned up. Do not repurpose a retired alias for manual use while it remains registered in this application's database.

Configuration is read on startup; there is no hot reload or admin list editor.

## Stateful filtering

Table removal prevents matches for new state creation according to your rules. Existing PF states may keep existing traffic alive. V1 does not kill states: an arbitrary alias may be used by several policies, and killing every state for an IP could disrupt unrelated services. Manage strict connection termination in your firewall policy and operations.

For IPv4 port forwarding, check that a NAT rule's built-in **Pass** option does not bypass a separate filter rule you intend to restrict with the alias.

References: [aliases](https://docs.opnsense.org/manual/aliases.html), [API authentication](https://docs.opnsense.org/development/how-tos/api.html), [firewall states](https://docs.opnsense.org/manual/firewall.html#states).
