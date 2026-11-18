import { useEffect, useState } from 'react'
import { LogOut, Moon, Sun } from 'lucide-react'
import type { Session } from './App'
import { dayLabel, relativeTime, t } from './i18n'
import { link, navigate, usePath } from './router'
import { setTheme, useTheme } from './theme'
import { Eyebrow, IconButton, Mark, cn, icon, pressable } from './ui'

type Inbox = { id: string; name: string; messageCount: number }

type Message = { id: string; receivedAt: string; subject: string; preview: string; sender: string; recipients: string[] }

const inboxPath = (inbox: Inbox) => `/inboxes/${inbox.id}`

async function getJSON<T>(path: string, onSessionEnd: () => void): Promise<T> {
  const response = await fetch(path)
  if (response.status === 401) onSessionEnd()
  if (!response.ok) throw new Error(response.statusText)
  return response.json()
}

export function Shell({ session, onSessionChange }: { session: Session; onSessionChange: () => Promise<void> }) {
  const path = usePath().replace(/\/+$/, '') || '/'
  const [inboxes, setInboxes] = useState<Inbox[]>()
  const [failed, setFailed] = useState(false)

  useEffect(() => {
    getJSON<{ inboxes: Inbox[] }>('/api/inboxes', onSessionChange).then(
      (body) => {
        setInboxes(body.inboxes)
        setFailed(false)
      },
      () => setFailed(true),
    )
  }, [path])

  useEffect(() => {
    if (path === '/' && inboxes?.[0]) navigate(inboxPath(inboxes[0]), 'replace')
  }, [path, inboxes])

  const selected = inboxes?.find((inbox) => inboxPath(inbox) === path)
  const signOut = () => fetch('/api/session', { method: 'DELETE' }).then(onSessionChange, onSessionChange)

  return (
    <div className="flex h-full overflow-hidden">
      <nav aria-label={t.inboxes} className="flex w-rail shrink-0 flex-col border-r border-line max-lg:hidden">
        <div className="flex h-bar shrink-0 items-center pl-gutter font-semibold">{t.inboxes}</div>
        <ul className="flex min-h-0 flex-1 flex-col gap-0.5 overflow-y-auto px-2 py-1">
          {inboxes?.map((inbox) => (
            <li key={inbox.id}>
              <a
                {...link(inboxPath(inbox))}
                aria-current={inbox.id === selected?.id ? 'page' : undefined}
                className={cn(pressable, 'flex h-8 items-center gap-2 px-2 font-medium aria-[current=page]:bg-selected aria-[current=page]:text-fg')}
              >
                <span className="truncate">{inbox.name}</span>
                <span className="ml-auto text-label font-normal text-fg-subtle tabular-nums">{inbox.messageCount}</span>
              </a>
            </li>
          ))}
        </ul>
        <Account session={session} onSignOut={signOut} className="shrink-0 border-t border-line py-1.5 pr-2 pl-gutter" />
      </nav>

      <section aria-label={t.messages} className="flex w-full min-w-0 flex-col md:w-list md:shrink-0">
        <div className="flex min-h-0 flex-1 flex-col">
          {selected && <MessageList key={selected.id} inbox={selected} onSessionEnd={onSessionChange} />}
          {failed && !inboxes && (
            <p role="alert" className="p-gutter">
              {t.failed}
            </p>
          )}
          {inboxes && !selected && path !== '/' && (
            <div className="flex flex-col items-start gap-2 p-gutter">
              <p className="font-medium">{t.inboxMissing}</p>
              {inboxes[0] && (
                <a {...link(inboxPath(inboxes[0]))} className="font-medium text-accent underline-offset-4 hover:underline max-md:py-3">
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

      <main className="my-2 mr-2 grid min-w-0 flex-1 place-items-center rounded-xl border border-line bg-surface-sunken max-md:hidden">
        <Mark decorative className="size-10 text-fg-disabled" />
      </main>
    </div>
  )
}

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

function useMinuteTick() {
  const [, setTick] = useState(0)
  useEffect(() => {
    const timer = setInterval(() => setTick((tick) => tick + 1), 60_000)
    return () => clearInterval(timer)
  }, [])
}

function MessageList({ inbox, onSessionEnd }: { inbox: Inbox; onSessionEnd: () => void }) {
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
        </div>
        <p className="mt-1 text-label text-fg-muted tabular-nums">{t.messageCount(inbox.messageCount)}</p>
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
                <Row key={message.id} message={message} />
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

const named = /^(.*) <([^<>]+)>$/

function senderLabel(sender: string) {
  const name = named.exec(sender)?.[1]
  return name && !/[<@]/.test(name) ? name : sender
}

const addressOf = (recipient: string) => named.exec(recipient)?.[2] ?? recipient

function Row({ message }: { message: Message }) {
  return (
    <li className="flex min-h-row flex-col justify-center gap-0.5 px-3 py-2.5">
      <span className="flex items-baseline gap-2">
        <span className={cn('truncate font-medium', !message.subject && 'text-fg-muted')}>{message.subject || t.noSubject}</span>
        <time dateTime={message.receivedAt} className="ml-auto shrink-0 text-caption text-fg-subtle tabular-nums">
          {relativeTime(new Date(message.receivedAt))}
        </time>
      </span>
      <span className="truncate text-label text-fg-muted">
        {senderLabel(message.sender)} → {addressOf(message.recipients[0] ?? '')}
      </span>
      <span className="truncate text-label text-fg-subtle">{message.preview}</span>
    </li>
  )
}
