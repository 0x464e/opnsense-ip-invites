import { useParams } from 'react-router-dom'
import { useMutation, useQuery } from '@tanstack/react-query'
import { ArrowRight, Check, Clock3, Globe, ShieldCheck } from 'lucide-react'
import { api, ApiError, date, duration } from './api'
import type { Authorization, Preview } from './api'
import { Brand, ErrorNotice, Footer, Spinner } from './components'

export default function InvitePage() {
  const { token } = useParams()
  const preview = useQuery({
    queryKey: ['invite', token],
    queryFn: () => api<Preview>(`/api/public/invites/${token}`),
  })
  const authorize = useMutation({
    mutationFn: () => api<Authorization>(`/api/public/invites/${token}/authorize`, {}),
  })
  const data = preview.data
  const success = authorize.data
  const errorCode = preview.error instanceof ApiError ? preview.error.code : ''
  const title =
    errorCode === 'expired'
      ? 'This invitation has expired'
      : errorCode === 'revoked'
        ? 'This invitation was revoked'
        : 'Invitation unavailable'
  return (
    <div className="flex min-h-dvh flex-col">
      <header className="mx-auto flex w-full max-w-6xl items-center justify-between px-6 py-7">
        <Brand />
        <span className="hidden text-xs text-muted sm:inline">Access by invitation</span>
      </header>
      <main className="flex flex-1 items-center justify-center px-5 py-10">
        <div className="w-full max-w-[460px]">
          <div className="panel overflow-hidden">
            {preview.isPending ? (
              <Spinner label="Checking your invitation" />
            ) : preview.error ? (
              <div className="p-8">
                <div className="mb-6 flex h-12 w-12 items-center justify-center rounded-2xl bg-paper text-muted">
                  <MailIcon />
                </div>
                <h1 className="text-2xl font-semibold tracking-tight">{title}</h1>
                <p className="mt-3 text-sm leading-relaxed text-muted">
                  {preview.error.message} Ask the person who shared it for a new link.
                </p>
              </div>
            ) : (
              data && (
                <>
                  <div className="p-7 sm:p-8">
                    <span className="tag tag-active">
                      <span className="h-1.5 w-1.5 rounded-full bg-leaf" />
                      {success ? 'Access authorized' : 'You’re invited'}
                    </span>
                    <div className="mt-7 flex h-13 w-13 items-center justify-center rounded-2xl border border-line bg-paper text-leaf">
                      {success ? <Check size={26} /> : <ShieldCheck size={26} strokeWidth={1.6} />}
                    </div>
                    <h1 className="mt-5 text-[28px] leading-tight font-semibold tracking-tight">
                      {success ? 'You’re all set.' : data.name}
                    </h1>
                    <p className="mt-3 text-sm leading-relaxed text-muted">
                      {success
                        ? 'Your connection has been authorized. You can return to the service you were invited to use.'
                        : data.description ||
                          'Authorize your current connection to access this service.'}
                    </p>
                    <div className="mt-7 rounded-xl border border-line bg-paper/60 px-4 py-4">
                      <div className="flex items-center gap-2 text-xs text-muted">
                        <Globe size={14} /> Your public IPv4
                      </div>
                      <p className="mt-2 font-mono text-xl tracking-tight">
                        {success?.ip || data.ipv4 || 'Unavailable'}
                      </p>
                    </div>
                    <div className="my-5 flex items-start gap-2 text-xs leading-relaxed text-muted">
                      <Clock3 size={15} className="mt-0.5 shrink-0" />
                      <span>
                        {success ? (
                          <>
                            Authorized until{' '}
                            <strong className="font-medium text-ink">
                              {date(success.expires_at)}
                            </strong>
                            {success.effective_expires_at > success.expires_at && (
                              <>
                                . Another invitation keeps your access active until{' '}
                                {date(success.effective_expires_at)}
                              </>
                            )}
                            .
                          </>
                        ) : (
                          <>
                            Each authorization lasts{' '}
                            <strong className="font-medium text-ink">
                              {duration(data.access_seconds)}
                            </strong>
                            . You can use this link until {date(data.link_expires_at)}.
                          </>
                        )}
                      </span>
                    </div>
                    {!success && data.ip_error && (
                      <div className="mb-5 space-y-3">
                        <ErrorNotice error={data.ip_error} />
                        {data.ipv4_url && (
                          <a className="btn w-full" href={data.ipv4_url}>
                            Continue over IPv4 <ArrowRight size={16} />
                          </a>
                        )}
                      </div>
                    )}
                    <ErrorNotice error={authorize.error} />
                    <button
                      className="btn btn-primary mt-4 min-h-12 w-full"
                      disabled={!data.ipv4 || authorize.isPending}
                      onClick={() => authorize.mutate()}
                    >
                      {authorize.isPending
                        ? 'Authorizing…'
                        : success
                          ? 'Renew my access'
                          : 'Authorize me'}
                      <ArrowRight size={17} />
                    </button>
                  </div>
                  <div className="border-t border-line bg-paper/40 px-7 py-4 text-xs leading-relaxed text-muted sm:px-8">
                    Use the same computer and internet connection as the service. If your IP
                    changes, reopen this link to authorize again.
                  </div>
                </>
              )
            )}
          </div>
          <p className="mt-5 text-center text-xs text-muted">
            Keep this invitation private. Anyone with the link can use it.
          </p>
        </div>
      </main>
      <Footer />
    </div>
  )
}
function MailIcon() {
  return <ShieldCheck size={25} strokeWidth={1.6} />
}
