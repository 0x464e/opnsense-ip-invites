# Reverse proxies and authentication

## Admin boundary

V1 delegates user login and authorization to your reverse proxy or identity provider. There are no built-in user accounts, passwords, or OIDC sessions.

Protect both `/admin` (including subpaths) and `/api/admin/`. After authentication succeeds, overwrite `X-Invites-Admin-Secret` with the same strong random value used by the backend's `INVITES_ADMIN_PROXY_SECRET`. Strip that header on public routes. Authenticate **before** injecting it.

The application checks the header using a constant-time comparison and also requires the connection peer to match `server.trusted_proxies`. It checks the Origin of admin mutations against configured public origins. This secret authenticates the proxy, not the user: the proxy must actually enforce which users are administrators. Never put the secret in browser JavaScript or public frontend environment variables.

Bind the backend to loopback or a private interface and restrict access to the actual reverse proxy. Trust only the smallest necessary CIDRs. Avoid broad private-network trust merely for convenience. Examples are templates, not a complete identity-provider setup:

- [Traefik dynamic configuration](../deploy/traefik.example.yaml)
- [nginx auth_request configuration](../deploy/nginx.example.conf)

For Docker, the immediate peer may be a bridge gateway rather than loopback. Inspect your actual deployment and configure its exact proxy address. HTTP between separate hosts should use a protected transport/network because both proxy secrets and visitor addresses cross that hop.

## Correct visitor addresses

Choose `server.client_ip_header`:

- Empty: use the TCP peer directly. Appropriate for an app directly behind a TLS terminator that preserves actual peer addresses, or direct networking setups.
- `X-Forwarded-For`: walk the chain from the verified peer right-to-left and stop at the first untrusted hop. Include legitimate intermediate proxies in `trusted_proxies`, and have the edge overwrite untrusted incoming forwarding headers.
- `CF-Connecting-IP`: use Cloudflare's visitor address through your own authenticated Cloudflare Tunnel/proxy chain. Preserve that header through the origin proxy and block direct, untrusted paths into the same trusted listener.
- `X-Real-IP`: use a single address set by your trusted proxy.

Forwarding headers from untrusted peers are ignored. Missing or malformed headers from a trusted peer fail rather than accidentally authorize the proxy's IP. Private, loopback, carrier-internal, IPv6, and synthetic Class E addresses are rejected.

With **Cloudflare → cloudflared → Traefik → app**, the app's trusted immediate peer is Traefik. It trusts Traefik to preserve the address supplied by your own tunnel. If Traefik also exposes an unrelated direct entry point, that route must overwrite/remove `CF-Connecting-IP` rather than allowing arbitrary client values into the trusted chain.

Cloudflare Workers can change forwarded IP behavior; do not add an unreviewed Worker intermediary. Cloudflare Pseudo IPv4 in Overwrite Headers mode replaces the real visitor IPv6 with a synthetic IPv4, which the app rejects. It cannot be used to discover the visitor's public IPv4.

## IPv4-only entry

An IPv4 backend or an IPv4 cloudflared tunnel does **not** make the browser's connection to Cloudflare IPv4-only. Cloudflare can accept IPv6 and forward its address to an IPv4-only origin.

For visitors who arrive over IPv6 but need IPv4 authorization, optionally set:

```yaml
server:
  # Other server fields omitted here.
  public_url: "https://invites.example.com"
  ipv4_public_url: "https://ipv4-invites.example.com"
```

The second origin must route to the **same app and database**, with a genuinely IPv4-only public observation path. For example, a DNS-only hostname with an A record and no AAAA record can point at an IPv4 HTTPS reverse proxy. That proxy must forward the actual visitor IPv4. Carrier translation can also deliver IPv4 to it; an IP-based allowlist still cannot distinguish subscribers sharing the same address or guarantee identical egress across different destinations.

The alternate origin needs a valid TLS certificate and the same admin protection if it exposes admin paths. Both configured origins are permitted by the app's mutation Origin checks. It does not automatically create DNS records, expose ports, change Cloudflare settings, or issue certificates.

A second ordinary Cloudflare-proxied hostname does not force IPv4. Blocking IPv6 HTTP requests also does not reliably make browsers retry over IPv4. If no alternate origin is configured, IPv6 visitors receive an explanation and cannot authorize. A visitor without IPv4 access or suitable carrier translation cannot use an IPv4-only service.

## Links and request logs

Invite tokens appear in `/i/{token}` and `/api/public/invites/{token}`. Treat full request paths as secrets. The application does not log request URLs; configure proxies, identity providers, CDN logging, and monitoring to avoid storing them. The nginx example excludes invite paths from its access log. For Traefik, omit request-path fields from access logs or disable logging for the relevant deployment. Do not collect Referer headers containing invitation URLs.

Responses use `no-store`, `no-referrer`, a same-origin content policy, and framing restrictions. Public authorization requires an explicit same-origin JSON POST; page visits and link previews cannot authorize by GET.

Health endpoints `/healthz` and `/readyz` are public and reveal only a status. Detailed list errors are admin-only.
