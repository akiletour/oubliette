import { useEffect, useState, type ReactNode } from 'react'
import { ChevronDown } from 'lucide-react'
import { relativeTime, t } from './i18n'
import { RequestFailed, fromTo, getJSON, messageAt, useMinuteTick, type OpenedMessage } from './messages'
import { link, navigate } from './router'
import { IconButton, cn, icon, textLink } from './ui'
import type { Place } from './Workspace'

const views = ['rendered', 'text'] as const

type View = (typeof views)[number]

export function Stage({ place, messageId, onSessionEnd }: { place: Place; messageId: string; onSessionEnd: () => void }) {
  const [opened, setOpened] = useState<OpenedMessage | 'missing' | 'failed'>()
  const [chosen, setChosen] = useState<View>()
  const api = messageAt(place.api, messageId)
  useMinuteTick()

  useEffect(() => {
    getJSON<OpenedMessage>(api, onSessionEnd).then(setOpened, (error) =>
      setOpened(error instanceof RequestFailed && error.status === 404 ? 'missing' : 'failed'),
    )
  }, [])

  if (opened === undefined) return null
  if (opened === 'missing' || opened === 'failed') {
    return (
      <Notice>
        <p className="font-medium">{opened === 'missing' ? t.messageMissing : t.messageFailed}</p>
        <a {...link(place.page)} className={textLink}>
          {t.goToInbox(place.name)}
        </a>
      </Notice>
    )
  }

  const view = chosen ?? (opened.hasHTML ? 'rendered' : 'text')
  const subject = opened.subject || t.noSubject
  const receivedAt = new Date(opened.receivedAt)

  return (
    <div className="relative flex min-w-0 flex-1 flex-col">
      <header className="flex shrink-0 items-start gap-2 px-3 pt-2 pb-3 max-md:flex-wrap md:px-6 md:pt-5 md:pb-4">
        <IconButton label={t.closeMessage} onClick={() => navigate(place.page)} className="md:hidden">
          <ChevronDown {...icon} />
        </IconButton>
        <div className="min-w-0 flex-1 max-md:basis-full max-md:px-2">
          <h2 className={cn('text-heading font-semibold max-md:line-clamp-2 md:truncate', !opened.subject && 'text-fg-muted')} title={subject}>
            {subject}
          </h2>
          <p className="mt-0.5 truncate text-label text-fg-muted">
            {fromTo(opened)} ·{' '}
            <time dateTime={opened.receivedAt}>{relativeTime(receivedAt)}</time>
          </p>
        </div>
      </header>

      {view === 'rendered' ? (
        <div className="flex min-h-0 flex-1 px-3 pb-18 md:px-6">
          <div className="flex-1 overflow-hidden rounded-lg bg-email-canvas shadow-raised md:rounded-xl">
            <iframe
              title={t.renderingOf(subject)}
              src={`${api}/rendered`}
              sandbox="allow-popups allow-popups-to-escape-sandbox"
              className="block size-full border-0 scheme-light"
            />
          </div>
        </div>
      ) : (
        <div className="mx-3 mb-18 min-h-0 flex-1 overflow-y-auto rounded-lg border border-line bg-surface p-4 md:mx-6 md:rounded-xl md:p-6">
          {opened.text ? (
            <pre className="max-w-reading font-sans text-reading wrap-anywhere whitespace-pre-wrap">{opened.text}</pre>
          ) : (
            <p className="text-fg-muted">{t.noTextVersion}</p>
          )}
        </div>
      )}

      <div className="pointer-events-none absolute inset-x-0 bottom-3 flex justify-center px-3">
        <div
          role="group"
          aria-label={t.messageViews}
          className="pointer-events-auto flex gap-0.5 rounded-full border border-line bg-surface-raised/85 p-1 shadow-raised backdrop-blur-md"
        >
          {views.map((key) => (
            <button
              key={key}
              type="button"
              aria-pressed={key === view}
              disabled={key === 'rendered' && !opened.hasHTML}
              onClick={() => setChosen(key)}
              className="relative h-8 rounded-full px-3 font-medium text-fg-muted transition-[color,background-color,scale] duration-(--duration-fast) ease-out before:absolute before:inset-x-0 before:-inset-y-1 hover:text-fg active:scale-[0.96] disabled:pointer-events-none disabled:text-fg-disabled aria-pressed:bg-selected aria-pressed:text-fg max-md:h-11 max-md:px-3.5 max-md:before:hidden"
            >
              {t[key]}
            </button>
          ))}
        </div>
      </div>
    </div>
  )
}

function Notice({ children }: { children: ReactNode }) {
  return <div className="flex flex-1 flex-col items-start gap-2 p-gutter md:p-6">{children}</div>
}
