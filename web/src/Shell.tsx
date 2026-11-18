import { useEffect, useRef, useState, useSyncExternalStore } from 'react'
import { ChevronsUpDown, LogOut, Moon, Pencil, Sun, Trash2 } from 'lucide-react'
import type { Session } from './App'
import { dayLabel, relativeTime, t } from './i18n'
import { DeleteInboxDialog, InboxNameDialog, InboxNav, SwitchInboxDialog } from './Inboxes'
import { fromTo, getJSON, inboxPath, messagePath, request, useMinuteTick, type Inbox, type Message } from './messages'
import { link, navigate, usePath } from './router'
import { Stage } from './Stage'
import { setTheme, useTheme } from './theme'
import { Eyebrow, IconButton, Mark, cn, icon, pressable, primaryButton, textLink } from './ui'

const route = /^\/inboxes\/([^/]+)(?:\/messages\/([^/]+))?$/

function useMediaQuery(query: string) {
  return useSyncExternalStore(
    (notify) => {
      const list = window.matchMedia(query)
      list.addEventListener('change', notify)
      return () => list.removeEventListener('change', notify)
    },
    () => window.matchMedia(query).matches,
  )
}

function useLastDefined<T>(value: T | undefined) {
  const last = useRef(value)
  if (value !== undefined) last.current = value
  return value ?? last.current
}

export function Shell({ session, onSessionChange }: { session: Session; onSessionChange: () => Promise<void> }) {
  const path = usePath().replace(/\/+$/, '') || '/'
  const [, inboxId, messageId] = route.exec(path) ?? []
  const [inboxes, setInboxes] = useState<Inbox[]>()
  const [failed, setFailed] = useState(false)
  const [dialog, setDialog] = useState<InboxDialog>()
  const wide = useMediaQuery('(min-width: 768px)')

  const loadInboxes = () =>
    getJSON<{ inboxes: Inbox[] }>('/api/inboxes', onSessionChange).then(
      (body) => {
        setInboxes(body.inboxes)
        setFailed(false)
      },
      () => setFailed(true),
    )

  useEffect(() => {
    loadInboxes()
  }, [inboxId])

  useEffect(() => {
    if (path === '/' && inboxes?.[0]) navigate(inboxPath(inboxes[0].id), 'replace')
  }, [path, inboxes])

  const selected = inboxes?.find((inbox) => inbox.id === inboxId)
  const open = selected !== undefined && messageId !== undefined
  const shown = useLastDefined(open ? messageId : undefined)
  const staged = wide ? (open ? messageId : undefined) : shown
  const signOut = () => fetch('/api/session', { method: 'DELETE' }).then(onSessionChange, onSessionChange)
  const closeDialog = (kind: InboxDialog) => () => setDialog((current) => (current === kind ? undefined : current))

  async function createInbox(name: string) {
    const created: Inbox = await (await request('POST', '/api/inboxes', { name }, onSessionChange)).json()
    await loadInboxes()
    navigate(inboxPath(created.id))
  }

  async function renameInbox(id: string, name: string) {
    await request('PATCH', `/api/inboxes/${id}`, { name }, onSessionChange)
    await loadInboxes()
  }

  async function deleteInbox(id: string) {
    await request('DELETE', `/api/inboxes/${id}`, undefined, onSessionChange)
    setDialog(undefined)
    setInboxes((listed) => listed?.filter((inbox) => inbox.id !== id))
    navigate('/', 'replace')
  }

  return (
    <div className="relative flex h-full overflow-hidden">
      <nav aria-label={t.inboxes} className="flex w-rail shrink-0 flex-col border-r border-line max-lg:hidden">
        <InboxNav inboxes={inboxes ?? []} selectedId={selected?.id} onCreate={() => setDialog('create')} />
        <Account session={session} onSignOut={signOut} className="shrink-0 border-t border-line py-1.5 pr-2 pl-gutter" />
      </nav>

      <section aria-label={t.messages} inert={!wide && open} className="flex w-full min-w-0 flex-col md:w-list md:shrink-0">
        <div className="flex min-h-0 flex-1 flex-col">
          {selected && <MessageList key={selected.id} inbox={selected} openId={messageId} onSessionEnd={onSessionChange} onAction={setDialog} />}
          {failed && !inboxes && (
            <p role="alert" className="p-gutter">
              {t.failed}
            </p>
          )}
          {inboxes?.length === 0 && (
            <div className="flex flex-col items-start gap-3 p-gutter">
              <div>
                <p className="font-medium">{t.noInbox}</p>
                <p className="mt-1 text-fg-muted">{t.noInboxHint}</p>
              </div>
              <button type="button" onClick={() => setDialog('create')} className={primaryButton}>
                {t.newInbox}
              </button>
            </div>
          )}
          {inboxes && inboxes.length > 0 && !selected && path !== '/' && (
            <div className="flex flex-col items-start gap-2 p-gutter">
              <p className="font-medium">{t.inboxMissing}</p>
              {inboxes[0] && (
                <a {...link(inboxPath(inboxes[0].id))} className={textLink}>
                  {t.goToInbox(inboxes[0].name)}
                </a>
              )}
            </div>
          )}
        </div>
        <footer className="flex shrink-0 items-center gap-2 border-t border-line py-1.5 pr-2 pl-gutter">
          <Account session={session} onSignOut={signOut} className="flex-1 lg:hidden" />
          <span className="ml-auto flex shrink-0">
            <ThemeToggle />
          </span>
        </footer>
      </section>

      <div
        aria-hidden
        data-open={open}
        className="pointer-events-none absolute inset-0 bg-scrim opacity-0 transition-opacity duration-(--duration-base) ease-out data-[open=true]:opacity-100 md:hidden"
      />
      <main
        data-open={open}
        inert={!wide && !open}
        className="relative flex min-w-0 flex-1 overflow-hidden bg-surface-sunken max-md:absolute max-md:inset-x-0 max-md:top-2 max-md:bottom-0 max-md:z-10 max-md:translate-y-full max-md:rounded-t-xl max-md:shadow-sheet max-md:transition-transform max-md:duration-(--duration-base) max-md:ease-out max-md:data-[open=true]:translate-y-0 max-md:data-[open=true]:duration-(--duration-slow) md:my-2 md:mr-2 md:rounded-xl md:border md:border-line"
      >
        {selected && staged ? (
          <Stage key={staged} inbox={selected} messageId={staged} onSessionEnd={onSessionChange} />
        ) : (
          <div className="grid flex-1 place-items-center">
            <Mark decorative className="size-10 text-fg-disabled" />
          </div>
        )}
      </main>

      {dialog === 'create' && <InboxNameDialog title={t.newInbox} action={t.create} onSubmit={createInbox} onClose={closeDialog('create')} />}
      {dialog === 'rename' && selected && (
        <InboxNameDialog
          title={t.renameInbox}
          action={t.rename}
          initial={selected.name}
          onSubmit={(name) => renameInbox(selected.id, name)}
          onClose={closeDialog('rename')}
        />
      )}
      {dialog === 'delete' && selected && (
        <DeleteInboxDialog inbox={selected} onDelete={() => deleteInbox(selected.id)} onClose={closeDialog('delete')} />
      )}
      {dialog === 'switch' && inboxes && (
        <SwitchInboxDialog inboxes={inboxes} selectedId={selected?.id} onCreate={() => setDialog('create')} onClose={closeDialog('switch')} />
      )}
    </div>
  )
}

type InboxDialog = 'create' | 'rename' | 'delete' | 'switch'

function Account({ session, onSignOut, className }: { session: Session; onSignOut: () => void; className?: string }) {
  return (
    <div className={cn('flex min-w-0 items-center gap-2', className)}>
      <span className="truncate text-label text-fg-muted" title={session.email}>
        {session.email}
      </span>
      <IconButton label={t.signOut} onClick={onSignOut} className="ml-auto">
        <LogOut {...icon} />
      </IconButton>
    </div>
  )
}

function ThemeToggle() {
  const theme = useTheme()
  const next = theme === 'dark' ? 'light' : 'dark'
  return (
    <IconButton label={next === 'dark' ? t.toDarkTheme : t.toLightTheme} onClick={() => setTheme(next)}>
      {theme === 'dark' ? <Sun {...icon} /> : <Moon {...icon} />}
    </IconButton>
  )
}

function MessageList({
  inbox,
  openId,
  onSessionEnd,
  onAction,
}: {
  inbox: Inbox
  openId?: string
  onSessionEnd: () => void
  onAction: (action: InboxDialog) => void
}) {
  const [messages, setMessages] = useState<Message[]>([])
  const [next, setNext] = useState('')
  const [status, setStatus] = useState<'loading' | 'ready' | 'failed'>('loading')
  useMinuteTick()

  async function load(before: string) {
    setStatus('loading')
    try {
      const page = await getJSON<{ messages: Message[]; next: string }>(
        `/api/inboxes/${inbox.id}/messages?before=${encodeURIComponent(before)}`,
        onSessionEnd,
      )
      setMessages((held) => (before ? [...held, ...page.messages] : page.messages))
      setNext(page.next)
      setStatus('ready')
    } catch {
      setStatus('failed')
    }
  }

  useEffect(() => {
    load('')
  }, [])

  return (
    <>
      <header className="shrink-0 px-gutter pt-5 pb-3">
        <div className="flex items-center gap-2.5">
          <Mark />
          <h1 className="truncate text-heading font-semibold">{inbox.name}</h1>
          <IconButton label={t.switchInbox} onClick={() => onAction('switch')} className="-ml-1 lg:hidden">
            <ChevronsUpDown {...icon} />
          </IconButton>
        </div>
        <div className="mt-1 flex items-center">
          <p className="text-label text-fg-muted tabular-nums">{t.messageCount(inbox.messageCount)}</p>
          <div className="-my-1.5 ml-auto flex">
            <IconButton label={t.renameInbox} onClick={() => onAction('rename')}>
              <Pencil {...icon} />
            </IconButton>
            <IconButton label={t.deleteInbox} onClick={() => onAction('delete')}>
              <Trash2 {...icon} />
            </IconButton>
          </div>
        </div>
      </header>
      <div className="min-h-0 flex-1 overflow-y-auto px-2 pb-3">
        {status === 'failed' && (
          <p role="alert" className="p-3">
            {t.loadFailed}
          </p>
        )}
        {status === 'ready' && messages.length === 0 && (
          <div className="p-3">
            <p className="font-medium">{t.emptyInbox}</p>
            <p className="mt-1 text-fg-muted">{t.emptyInboxHint}</p>
          </div>
        )}
        {groupByDay(messages).map(([day, received]) => (
          <section key={day} aria-label={day}>
            <Eyebrow className="px-3 pt-3 pb-1.5">{day}</Eyebrow>
            <ul className="flex flex-col gap-0.5">
              {received.map((message) => (
                <Row key={message.id} message={message} path={messagePath(inbox.id, message.id)} selected={message.id === openId} />
              ))}
            </ul>
          </section>
        ))}
        {next && (
          <button
            type="button"
            onClick={() => load(next)}
            disabled={status === 'loading'}
            className={cn(pressable, 'relative mt-2 h-8 w-full text-label font-medium before:absolute before:inset-x-0 before:-inset-y-1 max-md:h-11 max-md:before:hidden')}
          >
            {t.olderMessages}
          </button>
        )}
      </div>
    </>
  )
}

function groupByDay(messages: Message[]) {
  const days = new Map<string, Message[]>()
  for (const message of messages) {
    const day = dayLabel(new Date(message.receivedAt))
    days.set(day, [...(days.get(day) ?? []), message])
  }
  return [...days]
}

function Row({ message, path, selected }: { message: Message; path: string; selected: boolean }) {
  return (
    <li>
      <a
        {...link(path)}
        aria-current={selected ? 'true' : undefined}
        className="flex min-h-row flex-col justify-center gap-0.5 rounded-md px-3 py-2.5 transition-[background-color] duration-(--duration-fast) ease-out hover:bg-hover aria-[current=true]:bg-selected"
      >
        <span className="flex items-baseline gap-2">
          <span className={cn('truncate font-medium', !message.subject && 'text-fg-muted')}>{message.subject || t.noSubject}</span>
          <time dateTime={message.receivedAt} className="ml-auto shrink-0 text-caption text-fg-subtle tabular-nums">
            {relativeTime(new Date(message.receivedAt))}
          </time>
        </span>
        <span className="truncate text-label text-fg-muted">
          {fromTo(message)}
        </span>
        <span className="truncate text-label text-fg-subtle">{message.preview}</span>
      </a>
    </li>
  )
}
