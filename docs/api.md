# HTTP API

Same-origin JSON API used by the frontend. Timestamps are Unix seconds. Responses are not cached. Errors use `{ "code": "...", "message": "..." }`. All POST requests require `Content-Type: application/json` and an Origin equal to a configured public URL. Unknown JSON fields and trailing JSON documents are rejected. There is no cross-origin CORS API.

## Public

| Method | Path | Behavior |
|---|---|---|
| GET | `/api/public/invites/{token}` | Validate link; return list display text, access duration, detected IPv4 or an IP error, and optional IPv4 entry URL. Does not grant access. |
| POST | `/api/public/invites/{token}/authorize` | Send `{}`; authorize only the observed public IPv4. Return `status`, `ip`, `expires_at`, and `effective_expires_at` after confirmation. |
| GET | `/healthz` | Liveness. |
| GET | `/readyz` | Readiness; 503 while any registered alias is unconfirmed. |

Invite tokens have 256 bits of randomness and are passed as URL path components. Public invite calls are limited to 30 per minute per detected source address. An IPv6 or unavailable address yields an explanation on preview and HTTP 422 on authorization. An invalid link returns 404, an expired/revoked link 410, and a full invitation 409.

If a grant is recorded but firewall confirmation fails, authorization returns **503 `sync_pending`**. The stored desired grant is retried in the background. Never treat this response as confirmed access. Retrying the same IP renews that invitation's existing grant rather than creating a duplicate.

## Admin

Every admin call requires the trusted immediate proxy and `X-Invites-Admin-Secret`, injected only after that proxy authenticates the operator. These are server-to-server credentials, not a browser login API.

| Method | Path | Behavior |
|---|---|---|
| GET | `/api/admin/bootstrap` | Database-backed lists, sync statuses, and YAML-derived defaults in seconds. No credentials. |
| POST | `/api/admin/lists` | Create a list from `name`, optional `description` and optional `alias`; generates IDs and omitted alias names. Returns the list with 201, or 202 if synchronization is pending. |
| POST | `/api/admin/lists/{id}` | Edit `name` and `description`; alias changes are rejected. |
| POST | `/api/admin/lists/{id}/delete` | Delete the list, revoke its links/grants, and reconcile its retired alias. |
| GET | `/api/admin/links` | Paginated invitation metadata, without raw tokens or hashes. |
| POST | `/api/admin/links` | Create an invitation and return its metadata plus the shareable URL once. |
| POST | `/api/admin/links/{id}/revoke` | Disable link; `{"revoke_grants":true}` also revokes its grants. Omitted flag defaults to true. |
| GET | `/api/admin/grants` | Paginated grants with individual and effective list/IP expiry. |
| POST | `/api/admin/grants/{id}/revoke` | Send `{}` to revoke one grant. |
| POST | `/api/admin/sync` | Send `{}` to retry reconciliation immediately. |

Creation body:

```json
{
  "list_id": "list-ID_FROM_BOOTSTRAP",
  "label": "Weekend group",
  "expires_at": 1900000000,
  "access_seconds": 86400,
  "max_ips": 0
}
```

Only `list_id` and `label` are required. Omitted expiry/access/IP-limit values come from YAML defaults. Explicit `max_ips: 0` means unlimited. Labels are 1–100 bytes after trimming; grant duration is 1 minute to 10 years; link expiry must be in the future and within 10 years; maximum IP limit is 100000. Tokens are stored only as SHA-256 hashes, so the URL cannot be recovered later. Create a replacement if it is lost.

List queries accept `status=active|expired|revoked|all` (default active), `page` (default 1), and `page_size` (default 25, maximum 100). Response: `{ "items": [], "total": 0, "page": 1, "page_size": 25 }`.

A successful revocation returns `{"status":"synced"}`. A saved revocation with unconfirmed remote synchronization returns HTTP 202 with `status: pending`. Other active grants can keep the IP in the alias; an effective expiry refers to the entire list/IP, not just the row. A link can be revoked while its existing grants remain active when the operator chooses not to revoke them.
