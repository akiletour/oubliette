import { useEffect, useRef, useState, useSyncExternalStore, type ReactNode } from 'react'
import { dayLabel, relativeTime, t } from './i18n'
import { fromTo, getJSON, messageAt, useMinuteTick, type Message } from './messages'
import { link, navigate } from './router'
import { Stage, type Width } from './Stage'
import { Eyebrow, FilterField, Mark, cn, pressable, textLink } from './ui'

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
  onDeleteMessage,
  inspectorWhenRoomy,
  footer,
  children,
}: {
  place?: Place
  messageId?: string
  onSessionEnd: () => void
  onDeleteMessage?: (messageId: string) => Promise<unknown>
  inspectorWhenRoomy?: boolean
  footer: ReactNode
  children: ReactNode
}) {
  const wide = useMediaQuery('(min-width: 768px)')
  const roomy = useMediaQuery('(min-width: 1280px)')
  const [chosenWidth, setChosenWidth] = useState<Width>()
  const width = chosenWidth ?? (wide ? 'desktop' : 'mobile')
  const [inspectorChoice, setInspectorChoice] = useState<boolean>()
  const inspector = inspectorChoice ?? (roomy && inspectorWhenRoomy === true)
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
          <Stage
            key={staged}
            place={place}
            messageId={staged}
            onSessionEnd={onSessionEnd}
            onDelete={onDeleteMessage && (() => onDeleteMessage(staged))}
            preview={{ width, onWidth: setChosenWidth, inspector, onInspector: setInspectorChoice }}
          />
        ) : (
          <div className="grid flex-1 place-items-center">
            <Mark decorative className="size-10 text-fg-disabled" />
          </div>
        )}
      </main>
    </>
  )
}

type Page = { messages: Message[]; next: string }

const filterDelay = 200
const reconnectDelay = 5_000
const freshFor = 6_000

function useDebounced<T>(value: T, delay: number) {
  const [settled, setSettled] = useState(value)
  useEffect(() => {
    const timer = setTimeout(() => setSettled(value), delay)
    return () => clearTimeout(timer)
  }, [value, delay])
  return settled
}

const liveEvents = ['received', 'removed', 'emptied'] as const

type LiveEvent = { name: (typeof liveEvents)[number]; id?: string }

function useLiveInbox(api: string, onChange: (event?: LiveEvent) => void) {
  const [live, setLive] = useState(true)
  const [attempt, setAttempt] = useState(0)
  const latestOnChange = useRef(onChange)
  useEffect(() => {
    latestOnChange.current = onChange
  })
  useEffect(() => {
    let reopened = attempt > 0
    let retry: ReturnType<typeof setTimeout> | undefined
    const source = new EventSource(`${api}/events`)
    source.onopen = () => {
      setLive(true)
      if (reopened) latestOnChange.current()
      reopened = true
    }
    for (const name of liveEvents) {
      source.addEventListener(name, (event) => latestOnChange.current({ name, id: JSON.parse(event.data).id }))
    }
    source.onerror = () => {
      setLive(false)
      if (source.readyState !== EventSource.CLOSED) return
      latestOnChange.current()
      retry = setTimeout(() => setAttempt((count) => count + 1), reconnectDelay)
    }
    return () => {
      source.close()
      clearTimeout(retry)
    }
  }, [api, attempt])
  return live
}

function withFirstPage(held: Page, page: Page): Page {
  const last = page.messages.at(-1)
  const at = last && page.next ? held.messages.findIndex((message) => message.id === last.id) : -1
  return at === -1 ? page : { messages: [...page.messages, ...held.messages.slice(at + 1)], next: held.next }
}

export function MessageList({
  place,
  openId,
  onSessionEnd,
  onLiveChange,
  edits = 0,
  nameAction,
  actions,
  note,
  emptyHint,
}: {
  place: Place
  openId?: string
  onSessionEnd: () => void
  onLiveChange?: () => void
  edits?: number
  nameAction?: ReactNode
  actions?: ReactNode
  note?: string
  emptyHint: string
}) {
  const [list, setList] = useState<Page>({ messages: [], next: '' })
  const [status, setStatus] = useState<'loading' | 'ready' | 'failed'>('loading')
  const [query, setQuery] = useState('')
  const filter = useDebounced(query.trim(), filterDelay)
  const [fresh, setFresh] = useState<ReadonlySet<string>>(new Set())
  const lastLoad = useRef(0)
  useMinuteTick()

  async function load(before: string, refresh = false) {
    const call = ++lastLoad.current
    if (!refresh) setStatus('loading')
    try {
      const page = await getJSON<Page>(`${place.api}/messages?q=${encodeURIComponent(filter)}&before=${encodeURIComponent(before)}`, onSessionEnd)
      if (call !== lastLoad.current) return
      setList((held) => (before ? { messages: [...held.messages, ...page.messages], next: page.next } : refresh ? withFirstPage(held, page) : page))
      setStatus('ready')
    } catch {
      if (call === lastLoad.current) setStatus('failed')
    }
  }

  const live = useLiveInbox(place.api, (event) => {
    const id = event?.id
    if (event?.name === 'received' && id) {
      setFresh((ids) => new Set(ids).add(id))
      setTimeout(() => setFresh((ids) => new Set([...ids].filter((freshId) => freshId !== id))), freshFor)
    }
    if (event?.name === 'removed') {
      setList((held) => ({ ...held, messages: held.messages.filter((message) => message.id !== id) }))
    }
    if (openId && (event?.name === 'emptied' || (event?.name === 'removed' && id === openId))) navigate(place.page, 'replace')
    load('', true)
    onLiveChange?.()
  })

  useEffect(() => {
    load('')
  }, [filter])

  const seenEdits = useRef(edits)
  useEffect(() => {
    if (edits === seenEdits.current) return
    seenEdits.current = edits
    load('', true)
  }, [edits])

  return (
    <>
      <header className="shrink-0 px-gutter pt-5 pb-3">
        <div className="flex items-center gap-2.5">
          <Mark />
          <h1 className="truncate text-heading font-semibold">{place.name}</h1>
          {nameAction}
        </div>
        <div className="mt-1 flex min-h-5 items-center gap-3">
          <p className="truncate text-label text-fg-muted tabular-nums">
            {t.messageCount(place.messageCount)}
            {note && ` · ${note}`}
          </p>
          <LiveStatus live={live} />
          {actions && <div className="-my-1.5 ml-auto flex">{actions}</div>}
        </div>
        <FilterField label={t.filterMessages} value={query} onChange={setQuery} className="mt-3" />
      </header>
      <div className="min-h-0 flex-1 overflow-y-auto px-2 pb-3">
        {status === 'failed' && (
          <p role="alert" className="p-3">
            {t.loadFailed}
          </p>
        )}
        {status === 'ready' && list.messages.length === 0 && filter && (
          <div className="flex flex-col items-start gap-1 p-3">
            <p className="font-medium break-words">{t.noMessageMatches(filter)}</p>
            <p className="text-fg-muted">{t.filterHint}</p>
            <button type="button" onClick={() => setQuery('')} className={cn(textLink, 'mt-1')}>
              {t.clearFilter}
            </button>
          </div>
        )}
        {status === 'ready' && list.messages.length === 0 && !filter && (
          <div className="p-3">
            <p className="font-medium">{t.emptyInbox}</p>
            <p className="mt-1 text-fg-muted">{emptyHint}</p>
          </div>
        )}
        {groupByDay(list.messages).map(([day, received]) => (
          <section key={day} aria-label={day}>
            <Eyebrow className="px-3 pt-3 pb-1.5">{day}</Eyebrow>
            <ul className="flex flex-col gap-0.5">
              {received.map((message) => (
                <Row
                  key={message.id}
                  message={message}
                  path={messageAt(place.page, message.id)}
                  selected={message.id === openId}
                  fresh={fresh.has(message.id)}
                />
              ))}
            </ul>
          </section>
        ))}
        {list.next && (
          <button
            type="button"
            onClick={() => load(list.next)}
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

function LiveStatus({ live }: { live: boolean }) {
  return (
    <span role="status" title={live ? t.liveHint : undefined} className="flex shrink-0 items-center gap-1.5 text-label text-fg-muted">
      <span aria-hidden className={cn('size-1.5 rounded-full transition-[background-color] duration-(--duration-base) ease-out', live ? 'bg-positive' : 'bg-fg-disabled')} />
      {live ? t.live : t.reconnecting}
    </span>
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

function Row({ message, path, selected, fresh }: { message: Message; path: string; selected: boolean; fresh: boolean }) {
  return (
    <li className={cn('grid grid-rows-[1fr]', fresh && 'transition-[grid-template-rows] duration-(--duration-slow) ease-out starting:grid-rows-[0fr]')}>
      <div className="min-h-0 overflow-hidden">
        <a
          {...link(path)}
          aria-current={selected ? 'true' : undefined}
          className={cn(
            'flex min-h-row flex-col justify-center gap-0.5 rounded-md px-3 py-2.5 -outline-offset-2 transition-[background-color,translate,scale,opacity] ease-out hover:bg-hover aria-[current=true]:bg-selected',
            fresh ? 'duration-(--duration-slow) starting:-translate-y-2 starting:scale-[0.98] starting:opacity-0' : 'duration-(--duration-fast)',
          )}
        >
          <span className="flex items-baseline gap-2">
            <span className={cn('truncate font-medium', !message.subject && 'text-fg-muted')}>{message.subject || t.noSubject}</span>
            {fresh ? (
              <span className="ml-auto shrink-0 rounded-full bg-accent-soft px-1.5 text-caption font-medium text-accent">{t.fresh}</span>
            ) : (
              <time dateTime={message.receivedAt} className="ml-auto shrink-0 text-caption text-fg-subtle tabular-nums">
                {relativeTime(new Date(message.receivedAt))}
              </time>
            )}
          </span>
          <span className="truncate text-label text-fg-muted">{fromTo(message)}</span>
          <span className="truncate text-label text-fg-subtle">{message.preview}</span>
        </a>
      </div>
    </li>
  )
}
