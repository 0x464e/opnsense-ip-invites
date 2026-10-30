# Security

This application holds firewall API credentials and manages access-control data. Report suspected vulnerabilities privately to the repository owner rather than including live credentials or invitation links in public issues. A dedicated reporting channel can be configured after publication.

The intended boundary is:

- Reverse proxy authenticates and authorizes administrators.
- Backend verifies the trusted peer and proxy secret on every admin request.
- Configured trusted proxies supply the visitor's actual public IPv4.
- Public invitation tokens authorize only their holder's observed address, for a configured list and finite duration.
- Firewall rules remain operator-managed; the application only manages dedicated External alias contents.

API credentials remain on the server and use verified HTTPS. There is no insecure-TLS switch. Table-operation privileges can be broader than a single alias; network-restrict the management interface and use a dedicated API user. Avoid exposing backend or Vite development listeners publicly.

Treat invitation links as bearer credentials. They are reusable and may authorize multiple people, subject to per-link limits. Token hashes are stored instead of raw secrets, but tokens still occur in request paths and can leak through proxy/CDN logs, browser history, shared messages, screenshots, and backups of those systems. Configure logging accordingly. The application does not request or use third-party analytics.

SQLite holds link metadata and IP-address history. Keep the data directory and backups private. The app creates its database with mode 0600, creates new data directories with mode 0700, and takes an exclusive process lock. No automatic history retention is implemented in v1.

IP addresses do not identify individuals. Shared household/carrier NAT addresses can authorize other people sharing that egress; VPNs and destination-dependent routing can produce mismatches. There is no protection against vulnerabilities in the service you expose to an allowed address.

Expiration and revocation are reconciled by a running worker. App/API downtime can leave entries present past expiry. Existing PF states can outlive alias removal. V1 does not kill states or promise hard firewall-enforced expiry.

Only IPv4 is supported. IPv6 page visits through Cloudflare Tunnel do not reveal the visitor's IPv4, and Cloudflare Pseudo IPv4 is synthetic. Do not resolve that mismatch by allowing broad IPv4 ranges or trusting user-supplied addresses.
