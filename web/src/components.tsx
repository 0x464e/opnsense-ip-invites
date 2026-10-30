import { useEffect, useRef } from 'react'
import type { ReactNode } from 'react'
import { AlertCircle, LoaderCircle, Mail, X } from 'lucide-react'

export function Brand() {
  return (
    <div className="flex items-center gap-3">
      <div className="flex h-9 w-9 items-center justify-center rounded-xl bg-leaf text-white">
        <Mail size={19} strokeWidth={1.7} />
      </div>
      <div>
        <span className="text-sm font-semibold tracking-tight">
          OPNsense <span className="font-normal text-muted">IP Invites</span>
        </span>
      </div>
    </div>
  )
}
export function Footer() {
  return (
    <footer className="px-6 py-7 text-center text-xs text-muted">
      opnsense-ip-invites <span className="mx-2 text-line">/</span> Self-hosted access, by
      invitation.
    </footer>
  )
}
export function Spinner({ label = 'Loading' }: { label?: string }) {
  return (
    <div className="flex items-center justify-center gap-2 py-12 text-sm text-muted" role="status">
      <LoaderCircle className="animate-spin" size={18} />
      {label}
    </div>
  )
}
export function ErrorNotice({ error }: { error: Error | string | null }) {
  if (!error) return null
  return (
    <div
      className="flex items-start gap-2.5 rounded-lg border border-amber-200 bg-amber-50 p-3.5 text-sm leading-relaxed text-amber-950"
      role="alert"
    >
      <AlertCircle size={17} className="mt-0.5 shrink-0" />
      <span>{typeof error === 'string' ? error : error.message}</span>
    </div>
  )
}
export function Modal({
  title,
  children,
  onClose,
}: {
  title: string
  children: ReactNode
  onClose: () => void
}) {
  const ref = useRef<HTMLDialogElement>(null)
  useEffect(() => {
    ref.current?.showModal()
  }, [])
  return (
    <dialog
      ref={ref}
      aria-label={title}
      onClose={onClose}
      onClick={(event) => {
        if (event.target === event.currentTarget) onClose()
      }}
      className="panel overflow-y-auto p-0 text-ink"
    >
      <div className="flex items-center justify-between border-b border-line px-6 py-5">
        <h2 className="text-lg font-semibold tracking-tight">{title}</h2>
        <button className="btn-icon" aria-label="Close dialog" onClick={onClose}>
          <X size={18} />
        </button>
      </div>
      <div className="p-6">{children}</div>
    </dialog>
  )
}
