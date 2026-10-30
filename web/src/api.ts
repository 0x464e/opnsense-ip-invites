export class ApiError extends Error {
  readonly status: number
  readonly code: string
  constructor(status: number, code: string, message: string) {
    super(message)
    this.status = status
    this.code = code
  }
}

export async function api<T>(path: string, body?: unknown): Promise<T> {
  const response = await fetch(path, {
    method: body === undefined ? 'GET' : 'POST',
    headers: body === undefined ? {} : { 'Content-Type': 'application/json' },
    credentials: 'same-origin',
    cache: 'no-store',
    body: body === undefined ? undefined : JSON.stringify(body),
  })
  const data = await response.json().catch(() => null)
  if (!response.ok)
    throw new ApiError(
      response.status,
      data?.code || 'request_failed',
      data?.message || 'The server could not complete this request.',
    )
  return data as T
}

export interface AccessList {
  id: string
  name: string
  description: string
  alias: string
}
export interface SyncStatus {
  alias: string
  retired: boolean
  last_attempt: number
  last_success: number
  desired: number
  actual: number
  error: string
}
export interface Bootstrap {
  lists: AccessList[]
  statuses: SyncStatus[]
  defaults: { link_validity_seconds: number; access_seconds: number; max_ips: number }
}
export interface Invite {
  id: string
  list_id: string
  label: string
  created_at: number
  expires_at: number
  access_seconds: number
  max_ips: number
  revoked_at: number
  active_ips: number
}
export interface Grant {
  id: string
  link_id: string
  list_id: string
  label: string
  ip: string
  authorized_at: number
  expires_at: number
  revoked_at: number
  effective_expires_at: number
}
export interface Page<T> {
  items: T[]
  total: number
  page: number
  page_size: number
}
export interface Preview {
  name: string
  description: string
  link_expires_at: number
  access_seconds: number
  ipv4: string
  ip_error: string
  ipv4_url: string
}
export interface Authorization {
  status: string
  ip: string
  expires_at: number
  effective_expires_at: number
}
export interface MutationResult {
  status: string
  message?: string
}

export function date(seconds: number): string {
  return new Intl.DateTimeFormat(undefined, {
    month: 'short',
    day: 'numeric',
    hour: 'numeric',
    minute: '2-digit',
  }).format(new Date(seconds * 1000))
}
export function duration(seconds: number): string {
  if (seconds % 86400 === 0) return `${seconds / 86400} ${seconds === 86400 ? 'day' : 'days'}`
  if (seconds % 3600 === 0) return `${seconds / 3600} ${seconds === 3600 ? 'hour' : 'hours'}`
  return `${Math.round(seconds / 60)} minutes`
}
export function state(item: {
  revoked_at: number
  expires_at: number
}): 'active' | 'expired' | 'revoked' {
  return item.revoked_at ? 'revoked' : item.expires_at <= Date.now() / 1000 ? 'expired' : 'active'
}
