import { useState, type FormEvent, type MouseEvent } from 'react'
import { CircleAlert, Plus } from 'lucide-react'
import { t } from './i18n'
import { RequestFailed, inboxPath, type Inbox } from './messages'
import { link } from './router'
import { Dialog, FilterField, IconButton, cn, field, icon, pressable, primaryButton, secondaryButton } from './ui'

const folded = (text: string) => text.normalize('NFD').replace(/\p{M}/gu, '').toLowerCase()

export function InboxNav({
  inboxes,
  selectedId,
  onCreate,
  onNavigate,
}: {
  inboxes: Inbox[]
  selectedId?: string
  onCreate: () => void
  onNavigate?: () => void
}) {
  const [query, setQuery] = useState('')
  const shown = inboxes.filter((inbox) => folded(inbox.name).includes(folded(query.trim())))
  return (
    <>
      <div className="flex h-bar shrink-0 items-center justify-between pr-2 pl-gutter">
        <span className="font-semibold">{t.inboxes}</span>
        <IconButton label={t.newInbox} onClick={onCreate}>
          <Plus {...icon} />
        </IconButton>
      </div>
      <FilterField label={t.filterInboxes} value={query} onChange={setQuery} className="shrink-0 px-2 pb-2" />
      <ul className="flex min-h-0 flex-1 flex-col gap-0.5 overflow-y-auto px-2 py-1">
        {shown.map((inbox) => {
          const target = link(inboxPath(inbox.id))
          return (
            <li key={inbox.id}>
              <a
                href={target.href}
                onClick={(event: MouseEvent) => {
                  target.onClick(event)
                  onNavigate?.()
                }}
                aria-current={inbox.id === selectedId ? 'page' : undefined}
                className={cn(
                  pressable,
                  'flex h-8 items-center gap-2 px-2 font-medium aria-[current=page]:bg-selected aria-[current=page]:text-fg max-lg:h-11',
                )}
              >
                <span className="truncate">{inbox.name}</span>
                <span className="ml-auto text-label font-normal text-fg-subtle tabular-nums">{inbox.messageCount}</span>
              </a>
            </li>
          )
        })}
        {shown.length === 0 && query && <li className="px-2 py-1.5 text-fg-muted">{t.noInboxMatches}</li>}
      </ul>
    </>
  )
}

export function SwitchInboxDialog(props: { inboxes: Inbox[]; selectedId?: string; onCreate: () => void; onClose: () => void }) {
  return (
    <Dialog title={t.switchInbox} onClose={props.onClose}>
      {(close) => (
        <nav aria-label={t.inboxes} className="flex max-h-120 flex-col pb-2">
          <InboxNav
            inboxes={props.inboxes}
            selectedId={props.selectedId}
            onNavigate={close}
            onCreate={() => {
              close()
              props.onCreate()
            }}
          />
        </nav>
      )}
    </Dialog>
  )
}

function Refusal({ state }: { state: Submission }) {
  if (state !== 'failed' && state !== 'refused') return null
  return (
    <p role="alert" className="flex items-start gap-2">
      <CircleAlert {...icon} className="mt-0.5 shrink-0" />
      {state === 'refused' ? t.inboxNameRefused : t.failed}
    </p>
  )
}

type Submission = 'idle' | 'busy' | 'failed' | 'refused'

function useSubmission(action: () => Promise<unknown>, close: () => void) {
  const [state, setState] = useState<Submission>('idle')
  return [
    state,
    async (event: FormEvent) => {
      event.preventDefault()
      setState('busy')
      try {
        await action()
        close()
      } catch (error) {
        setState(error instanceof RequestFailed && error.status === 400 ? 'refused' : 'failed')
      }
    },
  ] as const
}

export function InboxNameDialog(props: {
  title: string
  action: string
  initial?: string
  onSubmit: (name: string) => Promise<unknown>
  onClose: () => void
}) {
  return (
    <Dialog title={props.title} onClose={props.onClose}>
      {(close) => <InboxNameForm {...props} close={close} />}
    </Dialog>
  )
}

function InboxNameForm({ title, action, initial = '', onSubmit, close }: Parameters<typeof InboxNameDialog>[0] & { close: () => void }) {
  const [name, setName] = useState(initial)
  const [state, submit] = useSubmission(() => onSubmit(name), close)
  return (
    <form onSubmit={submit} className="flex flex-col gap-4 p-5">
      <h2 className="text-heading font-semibold">{title}</h2>
      <label className="flex flex-col gap-1.5 text-label font-medium text-fg-muted">
        {t.inboxName}
        <input
          value={name}
          onChange={(event) => setName(event.target.value)}
          required
          spellCheck={false}
          autoComplete="off"
          className={field}
        />
      </label>
      <Refusal state={state} />
      <div className="flex justify-end gap-2">
        <button type="button" onClick={close} className={secondaryButton}>
          {t.cancel}
        </button>
        <button type="submit" disabled={state === 'busy' || !name.trim()} className={primaryButton}>
          {action}
        </button>
      </div>
    </form>
  )
}

export function DeleteInboxDialog({ inbox, onDelete, onClose }: { inbox: Inbox; onDelete: () => Promise<unknown>; onClose: () => void }) {
  const title = t.deleteInboxTitle(inbox.name)
  return (
    <Dialog title={title} onClose={onClose}>
      {(close) => <DeleteInboxForm title={title} count={inbox.messageCount} onDelete={onDelete} close={close} />}
    </Dialog>
  )
}

function DeleteInboxForm({ title, count, onDelete, close }: { title: string; count: number; onDelete: () => Promise<unknown>; close: () => void }) {
  const [state, submit] = useSubmission(onDelete, close)
  return (
    <form onSubmit={submit} className="flex flex-col gap-4 p-5">
      <h2 className="text-heading font-semibold text-balance">{title}</h2>
      <p className="text-fg-muted">{t.deleteInboxWarning(count)}</p>
      <Refusal state={state} />
      <div className="flex justify-end gap-2">
        <button type="button" onClick={close} className={secondaryButton}>
          {t.cancel}
        </button>
        <button type="submit" disabled={state === 'busy'} className={primaryButton}>
          {t.delete}
        </button>
      </div>
    </form>
  )
}
