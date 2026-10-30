import { useState } from 'react'
import { useMutation } from '@tanstack/react-query'
import { api } from './api'
import type { AccessList, MutationResult } from './api'
import { ErrorNotice, Modal } from './components'

type Props = { list?: AccessList; onClose: () => void; onSaved: (message?: string) => void }
export function ListEditor({ list, onClose, onSaved }: Props) {
  const [name, setName] = useState(list?.name || '')
  const [description, setDescription] = useState(list?.description || '')
  const [alias, setAlias] = useState(list?.alias || '')
  const save = useMutation({
    mutationFn: () =>
      api<MutationResult>(list ? `/api/admin/lists/${list.id}` : '/api/admin/lists', {
        name,
        description,
        alias,
      }),
    onSuccess: (result) => {
      onSaved(result.message || 'List updated.')
      onClose()
    },
  })
  return (
    <Modal title={list ? 'Edit list' : 'New list'} onClose={onClose}>
      <form
        className="space-y-5"
        onSubmit={(event) => {
          event.preventDefault()
          save.mutate()
        }}
      >
        <div>
          <label className="label" htmlFor="list-name">
            Name
          </label>
          <input
            autoFocus
            id="list-name"
            className="field"
            value={name}
            onChange={(e) => setName(e.target.value)}
            required
            maxLength={100}
          />
        </div>
        <div>
          <label className="label" htmlFor="list-description">
            Description
          </label>
          <textarea
            id="list-description"
            className="field"
            value={description}
            onChange={(e) => setDescription(e.target.value)}
            maxLength={500}
          />
        </div>
        <div>
          <label className="label" htmlFor="list-alias">
            OPNsense alias
          </label>
          <input
            id="list-alias"
            className="field font-mono"
            value={alias}
            onChange={(e) => setAlias(e.target.value)}
            readOnly={!!list}
            pattern="[A-Za-z][A-Za-z0-9_]*"
            maxLength={32}
            placeholder="Generated automatically if left empty"
          />
          <p className="hint">
            {list
              ? 'The alias stays fixed so existing firewall policies keep working.'
              : 'A missing alias is created automatically. An existing alias must be an empty, enabled External alias dedicated to this app.'}
          </p>
        </div>
        {!list && (
          <p className="text-xs leading-relaxed text-muted">
            After creation, reference the alias in your firewall policy and apply it in OPNsense.
            The app does not create or apply firewall rules.
          </p>
        )}
        <ErrorNotice error={save.error} />
        <div className="flex justify-end gap-2">
          <button type="button" className="btn" onClick={onClose}>
            Cancel
          </button>
          <button className="btn btn-primary" disabled={save.isPending}>
            {save.isPending ? 'Saving…' : list ? 'Save list' : 'Create list'}
          </button>
        </div>
      </form>
    </Modal>
  )
}
export function DeleteListDialog({ list, onClose, onSaved }: Props & { list: AccessList }) {
  const remove = useMutation({
    mutationFn: () => api<MutationResult>(`/api/admin/lists/${list.id}/delete`, {}),
    onSuccess: (result) => {
      onSaved(result.message || 'List deleted.')
      onClose()
    },
  })
  return (
    <Modal title="Delete list" onClose={onClose}>
      <p className="text-sm leading-relaxed text-muted">
        Delete <strong className="text-ink">{list.name}</strong> and revoke all its invitations and
        access grants? Its alias will be emptied after successful synchronization. The alias
        definition and firewall rules are retained.
      </p>
      <p className="hint mt-3">
        This cannot be undone. History is retained, and existing firewall connection states may
        survive removal.
      </p>
      <div className="mt-4">
        <ErrorNotice error={remove.error} />
      </div>
      <div className="mt-6 flex justify-end gap-2">
        <button className="btn" onClick={onClose}>
          Cancel
        </button>
        <button
          className="btn btn-danger"
          disabled={remove.isPending}
          onClick={() => remove.mutate()}
        >
          {remove.isPending ? 'Deleting…' : 'Delete list'}
        </button>
      </div>
    </Modal>
  )
}
