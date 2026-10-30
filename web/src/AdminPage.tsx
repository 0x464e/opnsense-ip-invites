import { useState } from 'react'
import type { FormEvent } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import {
  CircleSlash,
  Check,
  ChevronLeft,
  ChevronRight,
  Copy,
  Globe,
  Link2,
  ListFilter,
  Plus,
  RefreshCw,
  ShieldCheck,
  X,
} from 'lucide-react'
import { api, date, duration, state } from './api'
import type { Bootstrap, Grant, Invite, MutationResult, Page } from './api'
import { Brand, ErrorNotice, Footer, Modal, Spinner } from './components'

type Dialog =
  { kind: 'create' } | { kind: 'link'; invite: Invite } | { kind: 'grant'; grant: Grant } | null

export default function AdminPage() {
  const client = useQueryClient()
  const [tab, setTab] = useState<'links' | 'grants'>('links')
  const [filter, setFilter] = useState('active')
  const [page, setPage] = useState(1)
  const [dialog, setDialog] = useState<Dialog>(null)
  const [notice, setNotice] = useState('')
  const boot = useQuery({
    queryKey: ['admin', 'bootstrap'],
    queryFn: () => api<Bootstrap>('/api/admin/bootstrap'),
    refetchInterval: 15000,
  })
  const links = useQuery({
    queryKey: ['admin', 'links', filter, page],
    queryFn: () => api<Page<Invite>>(`/api/admin/links?status=${filter}&page=${page}`),
    enabled: tab === 'links',
    refetchInterval: 15000,
  })
  const grants = useQuery({
    queryKey: ['admin', 'grants', filter, page],
    queryFn: () => api<Page<Grant>>(`/api/admin/grants?status=${filter}&page=${page}`),
    enabled: tab === 'grants',
    refetchInterval: 15000,
  })
  const activeLinks = useQuery({
    queryKey: ['admin', 'active-links'],
    queryFn: () => api<Page<Invite>>('/api/admin/links?status=active&page_size=1'),
    refetchInterval: 15000,
  })
  const activeGrants = useQuery({
    queryKey: ['admin', 'active-grants'],
    queryFn: () => api<Page<Grant>>('/api/admin/grants?status=active&page_size=1'),
    refetchInterval: 15000,
  })
  const changed = (message?: string) => {
    setNotice(message || 'Changes saved.')
    void client.invalidateQueries({ queryKey: ['admin'] })
  }
  const sync = useMutation({
    mutationFn: () => api<MutationResult>('/api/admin/sync', {}),
    onSuccess: (result) => changed(result.message || 'All lists synchronized.'),
  })
  const rows = tab === 'links' ? links : grants
  const listName = (id: string) =>
    boot.data?.lists.find((list) => list.id === id)?.name || `${id} (removed)`
  const ready =
    boot.data &&
    boot.data.statuses.length > 0 &&
    boot.data.statuses.every((status) => !status.error && status.last_success)
  return (
    <div className="flex min-h-dvh flex-col">
      <header className="border-b border-line bg-white">
        <div className="mx-auto flex max-w-6xl items-center justify-between px-5 py-5 sm:px-8">
          <Brand />
          <span className="tag">
            <ShieldCheck size={13} />
            Administration
          </span>
        </div>
      </header>
      <main className="mx-auto w-full max-w-6xl flex-1 px-5 py-9 sm:px-8 sm:py-12">
        <div className="flex flex-wrap items-end justify-between gap-5">
          <div>
            <p className="eyebrow mb-2">Access management</p>
            <h1 className="text-3xl font-semibold tracking-tight">Invitations</h1>
            <p className="mt-2 text-sm text-muted">
              Share access. Keep control of who can connect.
            </p>
          </div>
          <button
            className="btn btn-primary"
            disabled={!boot.data}
            onClick={() => setDialog({ kind: 'create' })}
          >
            <Plus size={17} />
            New invitation
          </button>
        </div>
        {boot.error && (
          <div className="mt-6">
            <ErrorNotice error={boot.error} />
          </div>
        )}
        {notice && (
          <div
            role="status"
            className="mt-6 flex items-center justify-between gap-3 rounded-lg border border-line bg-white px-4 py-3 text-sm"
          >
            <span className="flex items-center gap-2">
              <Check size={16} className="text-leaf" />
              {notice}
            </span>
            <button className="btn-icon" aria-label="Dismiss notice" onClick={() => setNotice('')}>
              <X size={15} />
            </button>
          </div>
        )}
        <section aria-label="Overview" className="mt-8 grid grid-cols-1 gap-3 sm:grid-cols-3">
          <Metric
            label="Configured lists"
            value={boot.data?.lists.length}
            icon={<ListFilter size={19} />}
          />
          <Metric
            label="Active invitations"
            value={activeLinks.data?.total}
            icon={<Link2 size={19} />}
          />
          <Metric
            label="Active grants"
            value={activeGrants.data?.total}
            icon={<Globe size={19} />}
          />
        </section>
        <section aria-label="Lists" className="mt-8">
          <div className="mb-3 flex items-center justify-between gap-3">
            <h2 className="eyebrow">Your lists</h2>
            <button
              className="flex items-center gap-1.5 text-xs font-medium text-muted hover:text-ink disabled:opacity-50"
              disabled={sync.isPending || !boot.data}
              onClick={() => sync.mutate()}
            >
              <RefreshCw size={13} className={sync.isPending ? 'animate-spin' : ''} />
              {sync.isPending ? 'Synchronizing' : 'Sync now'}
            </button>
          </div>
          <ErrorNotice error={sync.error} />
          <div className="grid gap-3 md:grid-cols-2">
            {boot.data?.lists.map((list) => {
              const status = boot.data.statuses.find((item) => item.alias === list.alias)
              const ok = status?.last_success && !status.error
              return (
                <div className="panel px-5 py-4" key={list.id}>
                  <div className="flex items-start justify-between gap-3">
                    <div>
                      <h3 className="text-sm font-semibold">{list.name}</h3>
                      <p className="mt-1.5 font-mono text-[11px] text-muted">{list.alias}</p>
                    </div>
                    <span className={`tag ${ok ? 'tag-active' : 'tag-error'}`}>
                      <span
                        className={`h-1.5 w-1.5 rounded-full ${ok ? 'bg-leaf' : 'bg-amber-600'}`}
                      />
                      {ok
                        ? `${status.actual} IP${status.actual === 1 ? '' : 's'} · synced`
                        : 'Pending sync'}
                    </span>
                  </div>
                  {status?.error && (
                    <p className="mt-3 text-xs leading-relaxed text-amber-900">{status.error}</p>
                  )}
                </div>
              )
            })}
          </div>
          {boot.data?.statuses
            .filter((item) => item.retired)
            .map((item) => (
              <p className="mt-3 text-xs text-muted" key={item.alias}>
                Retired alias <code>{item.alias}</code>: {item.error || 'Old memberships cleared.'}
              </p>
            ))}
        </section>
        <section className="panel mt-8 overflow-hidden" aria-label="Invitations and grants">
          <div className="flex flex-wrap items-center justify-between gap-3 border-b border-line px-5 py-3">
            <div className="flex gap-1 rounded-lg bg-paper p-1">
              {(['links', 'grants'] as const).map((value) => (
                <button
                  key={value}
                  aria-pressed={tab === value}
                  className={`rounded-md px-3 py-2 text-xs font-medium ${tab === value ? 'bg-white text-ink shadow-xs' : 'text-muted'}`}
                  onClick={() => {
                    setTab(value)
                    setPage(1)
                  }}
                >
                  {value === 'links' ? 'Invitations' : 'Access grants'}
                </button>
              ))}
            </div>
            <label className="flex items-center gap-2 text-xs text-muted">
              <ListFilter size={14} />
              <span className="sr-only">Filter by status</span>
              <select
                aria-label="Filter by status"
                className="rounded-md border border-line bg-white px-2 py-1.5 text-xs text-ink"
                value={filter}
                onChange={(event) => {
                  setFilter(event.target.value)
                  setPage(1)
                }}
              >
                <option value="active">Active</option>
                <option value="expired">Expired</option>
                <option value="revoked">Revoked</option>
                <option value="all">All statuses</option>
              </select>
            </label>
          </div>
          {rows.isPending ? (
            <Spinner />
          ) : rows.error ? (
            <div className="p-5">
              <ErrorNotice error={rows.error} />
            </div>
          ) : !rows.data?.items.length ? (
            <div className="px-6 py-16 text-center">
              <div className="mx-auto mb-4 flex h-11 w-11 items-center justify-center rounded-xl border border-line bg-paper text-muted">
                {tab === 'links' ? <Link2 size={21} /> : <Globe size={21} />}
              </div>
              <h3 className="text-sm font-medium">
                {tab === 'links' ? 'No invitations here yet' : 'No access grants here yet'}
              </h3>
              <p className="mx-auto mt-2 max-w-sm text-sm leading-relaxed text-muted">
                {tab === 'links'
                  ? 'Create an invitation and share its link to let someone authorize their connection.'
                  : 'Addresses appear here when someone authorizes through an invitation.'}
              </p>
            </div>
          ) : (
            <div className="relative overflow-x-auto">
              {tab === 'links' ? (
                <table className="w-full min-w-[680px]">
                  <thead className="bg-paper/50">
                    <tr>
                      {['Invitation', 'List', 'Link expires', 'Active IPs', 'Status', ''].map(
                        (title, i) => (
                          <th className="th" key={i} scope="col">
                            {title || <span className="sr-only">Actions</span>}
                          </th>
                        ),
                      )}
                    </tr>
                  </thead>
                  <tbody className="divide-y divide-line">
                    {links.data?.items.map((invite) => (
                      <tr key={invite.id}>
                        <td className="td font-medium">
                          {invite.label}
                          <span className="mt-1 block text-xs font-normal text-muted">
                            {duration(invite.access_seconds)} per authorization
                          </span>
                        </td>
                        <td className="td text-muted">{listName(invite.list_id)}</td>
                        <td className="td whitespace-nowrap text-muted">
                          {date(invite.expires_at)}
                        </td>
                        <td className="td font-mono text-xs">
                          {invite.active_ips}
                          <span className="text-muted"> / {invite.max_ips || '∞'}</span>
                        </td>
                        <td className="td">
                          <StatusTag value={state(invite)} />
                        </td>
                        <td className="td">
                          <button
                            className="btn-icon"
                            disabled={!!invite.revoked_at && invite.active_ips === 0}
                            aria-label={`Revoke ${invite.label}`}
                            title="Revoke invitation"
                            onClick={() => setDialog({ kind: 'link', invite })}
                          >
                            <CircleSlash size={16} />
                          </button>
                        </td>
                      </tr>
                    ))}
                  </tbody>
                </table>
              ) : (
                <table className="w-full min-w-[720px]">
                  <thead className="bg-paper/50">
                    <tr>
                      {[
                        'IPv4 address',
                        'Invitation / list',
                        'Grant expires',
                        'Effective access until',
                        'Status',
                        '',
                      ].map((title, i) => (
                        <th className="th" key={i} scope="col">
                          {title || <span className="sr-only">Actions</span>}
                        </th>
                      ))}
                    </tr>
                  </thead>
                  <tbody className="divide-y divide-line">
                    {grants.data?.items.map((grant) => (
                      <tr key={grant.id}>
                        <td className="td font-mono text-xs">{grant.ip}</td>
                        <td className="td font-medium">
                          {grant.label}
                          <span className="mt-1 block text-xs font-normal text-muted">
                            {listName(grant.list_id)}
                          </span>
                        </td>
                        <td className="td whitespace-nowrap text-muted">
                          {date(grant.expires_at)}
                        </td>
                        <td className="td whitespace-nowrap text-muted">
                          {grant.effective_expires_at ? date(grant.effective_expires_at) : '—'}
                        </td>
                        <td className="td">
                          <StatusTag value={state(grant)} />
                        </td>
                        <td className="td">
                          <button
                            className="btn-icon"
                            disabled={state(grant) !== 'active'}
                            aria-label={`Revoke ${grant.ip} from ${grant.label}`}
                            title="Revoke grant"
                            onClick={() => setDialog({ kind: 'grant', grant })}
                          >
                            <CircleSlash size={16} />
                          </button>
                        </td>
                      </tr>
                    ))}
                  </tbody>
                </table>
              )}
            </div>
          )}
          <div className="flex items-center justify-between gap-3 border-t border-line px-5 py-3 text-xs text-muted">
            <span>
              {rows.data?.total || 0} {tab === 'links' ? 'invitations' : 'grants'}
            </span>
            <div className="flex items-center gap-2">
              <button
                className="btn-icon"
                aria-label="Previous page"
                disabled={page <= 1}
                onClick={() => setPage(page - 1)}
              >
                <ChevronLeft size={16} />
              </button>
              <span>Page {page}</span>
              <button
                className="btn-icon"
                aria-label="Next page"
                disabled={!rows.data || page * rows.data.page_size >= rows.data.total}
                onClick={() => setPage(page + 1)}
              >
                <ChevronRight size={16} />
              </button>
            </div>
          </div>
        </section>
        <p className="mt-5 flex items-start gap-2 text-xs leading-relaxed text-muted">
          <span
            className={`mt-1 h-1.5 w-1.5 shrink-0 rounded-full ${ready ? 'bg-leaf' : 'bg-amber-500'}`}
          />
          {ready ? 'Lists are synchronized.' : 'Waiting for all lists to synchronize.'} An address
          stays authorized while any active grant for its list requires it.
        </p>
      </main>
      <Footer />
      {dialog?.kind === 'create' && boot.data && (
        <CreateInvite
          bootstrap={boot.data}
          onClose={() => setDialog(null)}
          onCreated={() => changed('Invitation created.')}
        />
      )}
      {dialog?.kind === 'link' && (
        <RevokeDialog invite={dialog.invite} onClose={() => setDialog(null)} onChanged={changed} />
      )}
      {dialog?.kind === 'grant' && (
        <RevokeDialog grant={dialog.grant} onClose={() => setDialog(null)} onChanged={changed} />
      )}
    </div>
  )
}

function Metric({ label, value, icon }: { label: string; value?: number; icon: React.ReactNode }) {
  return (
    <div className="panel flex items-center justify-between px-5 py-5">
      <div>
        <p className="text-xs text-muted">{label}</p>
        <p className="mt-2 text-3xl font-medium tracking-tight">{value ?? '—'}</p>
      </div>
      <span className="flex h-10 w-10 items-center justify-center rounded-xl border border-line bg-paper text-muted">
        {icon}
      </span>
    </div>
  )
}
function StatusTag({ value }: { value: string }) {
  return <span className={`tag ${value === 'active' ? 'tag-active' : ''}`}>{value}</span>
}
function localDate(seconds: number): string {
  const value = new Date(seconds * 1000)
  return `${value.getFullYear()}-${String(value.getMonth() + 1).padStart(2, '0')}-${String(value.getDate()).padStart(2, '0')}T${String(value.getHours()).padStart(2, '0')}:${String(value.getMinutes()).padStart(2, '0')}`
}
function CreateInvite({
  bootstrap,
  onClose,
  onCreated,
}: {
  bootstrap: Bootstrap
  onClose: () => void
  onCreated: () => void
}) {
  const defaults = bootstrap.defaults
  const [listID, setListID] = useState(bootstrap.lists[0]?.id || '')
  const [label, setLabel] = useState('')
  const [expiry, setExpiry] = useState(
    localDate(Math.floor(Date.now() / 1000) + defaults.link_validity_seconds),
  )
  const initialUnit =
    defaults.access_seconds % 86400 === 0 ? 86400 : defaults.access_seconds % 3600 === 0 ? 3600 : 60
  const [unit, setUnit] = useState(initialUnit)
  const [access, setAccess] = useState(String(defaults.access_seconds / initialUnit))
  const [limit, setLimit] = useState(String(defaults.max_ips))
  const [copyState, setCopyState] = useState('')
  const create = useMutation({
    mutationFn: () =>
      api<{ link: Invite; url: string }>('/api/admin/links', {
        list_id: listID,
        label,
        expires_at: Math.floor(new Date(expiry).getTime() / 1000),
        access_seconds: Math.round(Number(access) * unit),
        max_ips: Number(limit),
      }),
    onSuccess: onCreated,
  })
  const submit = (event: FormEvent) => {
    event.preventDefault()
    create.mutate()
  }
  return (
    <Modal title={create.data ? 'Your invitation is ready' : 'New invitation'} onClose={onClose}>
      {create.data ? (
        <div>
          <div className="mb-5 flex h-12 w-12 items-center justify-center rounded-2xl bg-leaf-light text-leaf">
            <Check size={24} />
          </div>
          <p className="text-sm leading-relaxed text-muted">
            Share this link with the people you want to invite. Save it now — it is shown only once.
          </p>
          <label htmlFor="invite-url" className="label mt-5">
            Invitation link
          </label>
          <input
            id="invite-url"
            className="field font-mono text-xs"
            readOnly
            value={create.data.url}
            onFocus={(event) => event.target.select()}
          />
          <div className="mt-4 flex gap-2">
            <button
              className="btn btn-primary flex-1"
              onClick={async () => {
                try {
                  await navigator.clipboard.writeText(create.data!.url)
                  setCopyState('Copied to clipboard.')
                } catch {
                  setCopyState('Select the link above and copy it manually.')
                }
              }}
            >
              <Copy size={16} />
              Copy link
            </button>
            <button className="btn" onClick={onClose}>
              Done
            </button>
          </div>
          <p className="mt-3 text-xs text-muted" role="status">
            {copyState}
          </p>
        </div>
      ) : (
        <form onSubmit={submit} className="space-y-5">
          <div>
            <label className="label" htmlFor="invite-label">
              Label
            </label>
            <input
              autoFocus
              id="invite-label"
              className="field"
              placeholder="e.g. Weekend group"
              required
              maxLength={100}
              value={label}
              onChange={(event) => setLabel(event.target.value)}
            />
          </div>
          <div>
            <label className="label" htmlFor="invite-list">
              Access list
            </label>
            <select
              id="invite-list"
              className="field"
              value={listID}
              onChange={(event) => setListID(event.target.value)}
            >
              {bootstrap.lists.map((list) => (
                <option value={list.id} key={list.id}>
                  {list.name}
                </option>
              ))}
            </select>
          </div>
          <div>
            <label className="label" htmlFor="invite-expiry">
              Link expires
            </label>
            <input
              id="invite-expiry"
              className="field"
              type="datetime-local"
              required
              value={expiry}
              onChange={(event) => setExpiry(event.target.value)}
            />
            <p className="hint">Stops new authorizations. Existing access keeps its own expiry.</p>
          </div>
          <div>
            <label className="label" htmlFor="access-duration">
              Access per authorization
            </label>
            <div className="flex gap-2">
              <input
                id="access-duration"
                className="field"
                type="number"
                min={60 / unit}
                step="any"
                required
                value={access}
                onChange={(event) => setAccess(event.target.value)}
              />
              <select
                className="field max-w-32"
                aria-label="Access duration unit"
                value={unit}
                onChange={(event) => {
                  const next = Number(event.target.value)
                  setAccess(String((Number(access) * unit) / next))
                  setUnit(next)
                }}
              >
                <option value={60}>Minutes</option>
                <option value={3600}>Hours</option>
                <option value={86400}>Days</option>
              </select>
            </div>
          </div>
          <div>
            <label className="label" htmlFor="ip-limit">
              Active IP limit
            </label>
            <input
              id="ip-limit"
              className="field"
              type="number"
              min="0"
              max="100000"
              step="1"
              required
              value={limit}
              onChange={(event) => setLimit(event.target.value)}
            />
            <p className="hint">
              0 = unlimited. 1 = replace the previous IP when a new one authorizes. Higher limits
              retain existing IPs and reject new ones when full.
            </p>
          </div>
          <ErrorNotice error={create.error} />
          <div className="flex justify-end gap-2 border-t border-line pt-5">
            <button className="btn" type="button" onClick={onClose}>
              Cancel
            </button>
            <button className="btn btn-primary" disabled={create.isPending}>
              {create.isPending ? 'Creating…' : 'Create invitation'}
              <Plus size={16} />
            </button>
          </div>
        </form>
      )}
    </Modal>
  )
}
function RevokeDialog({
  invite,
  grant,
  onClose,
  onChanged,
}: {
  invite?: Invite
  grant?: Grant
  onClose: () => void
  onChanged: (message?: string) => void
}) {
  const [revokeGrants, setRevokeGrants] = useState(true)
  const revoke = useMutation({
    mutationFn: () =>
      invite
        ? api<MutationResult>(`/api/admin/links/${invite.id}/revoke`, {
            revoke_grants: revokeGrants,
          })
        : api<MutationResult>(`/api/admin/grants/${grant!.id}/revoke`, {}),
    onSuccess: (result) => {
      onChanged(result.message || (invite ? 'Invitation revoked.' : 'Access grant revoked.'))
      onClose()
    },
  })
  return (
    <Modal title={invite ? 'Revoke invitation' : 'Revoke access grant'} onClose={onClose}>
      <p className="text-sm leading-relaxed text-muted">
        {invite ? (
          <>
            Disable future authorizations through{' '}
            <strong className="text-ink">{invite.label}</strong>.
          </>
        ) : (
          <>
            Remove this grant for <strong className="font-mono text-ink">{grant?.ip}</strong> from{' '}
            {grant?.label}.
          </>
        )}
      </p>
      {invite && (
        <label className="mt-5 flex items-center gap-3 rounded-lg border border-line p-4 text-sm">
          <input
            type="checkbox"
            className="h-4 w-4 accent-leaf"
            checked={revokeGrants}
            onChange={(event) => setRevokeGrants(event.target.checked)}
          />
          Also revoke existing access
        </label>
      )}
      <p className="mt-4 text-xs leading-relaxed text-muted">
        An IP with another active grant for the same list stays authorized. Existing connections may
        continue after membership is removed.
      </p>
      <div className="mt-5">
        <ErrorNotice error={revoke.error} />
      </div>
      <div className="mt-6 flex justify-end gap-2">
        <button className="btn" onClick={onClose}>
          Cancel
        </button>
        <button
          className="btn btn-danger"
          disabled={revoke.isPending}
          onClick={() => revoke.mutate()}
        >
          {revoke.isPending ? 'Revoking…' : 'Revoke'}
        </button>
      </div>
    </Modal>
  )
}
