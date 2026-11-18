import { useEffect, useRef, useState, useSyncExternalStore, type ReactNode } from 'react'
import { dayLabel, relativeTime, t } from './i18n'
import { fromTo, getJSON, messageAt, useMinuteTick, type Message } from './messages'
import { link } from './router'
import { Stage } from './Stage'
import { Eyebrow, Mark, cn, pressable } from './ui'

export type Place = { key: string; name: string; messageCount: number; api: string; page: string }

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

export function Workspace({
  place,
  messageId,
  onSessionEnd,
  footer,
  children,
}: {
  place?: Place
  messageId?: string
  onSessionEnd: () => void
  footer: ReactNode
  children: ReactNode
}) {
  const wide = useMediaQuery('(min-width: 768px)')
  const open = place !== undefined && messageId !== undefined
  const shown = useLastDefined(open ? messageId : undefined)
  const staged = wide ? (open ? messageId : undefined) : shown
  return (
    <>
      <section aria-label={t.messages} inert={!wide && open} className="flex w-full min-w-0 flex-col md:w-list md:shrink-0">
        <div className="flex min-h-0 flex-1 flex-col">{children}</div>
        <footer className="flex shrink-0 items-center gap-2 border-t border-line py-1.5 pr-2 pl-gutter">{footer}</footer>
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
        {place && staged ? (
          <Stage key={staged} place={place} messageId={staged} onSessionEnd={onSessionEnd} />
        ) : (
          <div className="grid flex-1 place-items-center">
            <Mark decorative className="size-10 text-fg-disabled" />
          </div>
        )}
      </main>
    </>
  )
}

export function MessageList({
  place,
  openId,
  onSessionEnd,
  nameAction,
  actions,
  note,
  emptyHint,
}: {
  place: Place
  openId?: string
  onSessionEnd: () => void
  nameAction?: ReactNode
  actions?: ReactNode
  note?: string
  emptyHint: string
}) {
  const [messages, setMessages] = useState<Message[]>([])
  const [next, setNext] = useState('')
  const [status, setStatus] = useState<'loading' | 'ready' | 'failed'>('loading')
  useMinuteTick()

  async function load(before: string) {
    setStatus('loading')
    try {
      const page = await getJSON<{ messages: Message[]; next: string }>(`${place.api}/messages?before=${encodeURIComponent(before)}`, onSessionEnd)
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
          <h1 className="truncate text-heading font-semibold">{place.name}</h1>
          {nameAction}
        </div>
        <div className="mt-1 flex min-h-5 items-center">
          <p className="text-label text-fg-muted tabular-nums">
            {t.messageCount(place.messageCount)}
            {note && ` · ${note}`}
          </p>
          {actions && <div className="-my-1.5 ml-auto flex">{actions}</div>}
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
            <p className="mt-1 text-fg-muted">{emptyHint}</p>
          </div>
        )}
        {groupByDay(messages).map(([day, received]) => (
          <section key={day} aria-label={day}>
            <Eyebrow className="px-3 pt-3 pb-1.5">{day}</Eyebrow>
            <ul className="flex flex-col gap-0.5">
              {received.map((message) => (
                <Row key={message.id} message={message} path={messageAt(place.page, message.id)} selected={message.id === openId} />
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
        <span className="truncate text-label text-fg-muted">{fromTo(message)}</span>
        <span className="truncate text-label text-fg-subtle">{message.preview}</span>
      </a>
    </li>
  )
}
