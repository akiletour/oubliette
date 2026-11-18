import { useEffect, useLayoutEffect, useRef, useState, type ReactNode } from 'react'
import { ChevronDown, Download, FileText, Info, Monitor, PanelRight, Smartphone, Tablet, Trash2, X } from 'lucide-react'
import { formatSize, fullDate, relativeTime, t } from './i18n'
import { RequestFailed, fromTo, getJSON, messageAt, request, useMinuteTick, type OpenedMessage } from './messages'
import { link, navigate } from './router'
import { Eyebrow, IconButton, Refusal, cn, icon, iconButton, textLink, useSubmission } from './ui'
import type { Place } from './Workspace'

const views = ['rendered', 'text', 'source', 'raw'] as const

type View = (typeof views)[number]

export type Width = 'mobile' | 'tablet' | 'desktop'

export type Preview = {
  width: Width
  onWidth: (width: Width) => void
  inspector: boolean
  onInspector: (open: boolean) => void
}

export function Stage({
  place,
  messageId,
  onSessionEnd,
  onDelete,
  preview,
}: {
  place: Place
  messageId: string
  onSessionEnd: () => void
  onDelete?: () => Promise<unknown>
  preview: Preview
}) {
  const [opened, setOpened] = useState<OpenedMessage | 'missing' | 'failed'>()
  const [chosen, setChosen] = useState<View>()
  const [sheetOpen, setSheetOpen] = useState(false)
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

  const view = chosen ?? (opened.html ? 'rendered' : 'text')
  const subject = opened.subject || t.noSubject
  const receivedAt = new Date(opened.receivedAt)

  return (
    <div className="relative flex min-w-0 flex-1">
      <div className="relative flex min-w-0 flex-1 flex-col">
        <header className="flex shrink-0 items-start gap-2 px-3 pt-2 pb-3 max-md:flex-wrap max-md:justify-between md:px-6 md:pt-5 md:pb-4">
          <IconButton label={t.closeMessage} onClick={() => navigate(place.page)} className="md:hidden">
            <ChevronDown {...icon} />
          </IconButton>
          <div className="min-w-0 flex-1 max-md:order-last max-md:basis-full max-md:px-2">
            <h2 className={cn('text-heading font-semibold max-md:line-clamp-2 md:truncate', !opened.subject && 'text-fg-muted')} title={subject}>
              {subject}
            </h2>
            <p className="mt-0.5 truncate text-label text-fg-muted">
              {fromTo(opened)} ·{' '}
              <time dateTime={opened.receivedAt}>{relativeTime(receivedAt)}</time>
            </p>
          </div>
          {view === 'rendered' && <WidthToggle value={preview.width} onChange={preview.onWidth} className="ml-auto md:hidden" />}
          <IconButton
            label={preview.inspector ? t.hideDetails : t.showDetails}
            aria-pressed={preview.inspector}
            onClick={() => preview.onInspector(!preview.inspector)}
            className="aria-pressed:bg-selected aria-pressed:text-fg max-md:hidden"
          >
            <PanelRight {...icon} />
          </IconButton>
          {onDelete && <DeleteButton onDelete={onDelete} />}
        </header>

        {view === 'rendered' ? (
          <EmailFrame src={`${api}/rendered`} title={t.renderingOf(subject)} width={preview.width} />
        ) : (
          <div className="mx-3 mb-18 min-h-0 flex-1 overflow-y-auto rounded-lg border border-line bg-surface p-4 md:mx-6 md:rounded-xl md:p-6">
            {view === 'text' &&
              (opened.text ? (
                <pre className="max-w-reading font-sans text-reading wrap-anywhere whitespace-pre-wrap">{opened.text}</pre>
              ) : (
                <p className="text-fg-muted">{t.noTextVersion}</p>
              ))}
            {view === 'source' && <Code>{opened.html}</Code>}
            {view === 'raw' && <RawSource api={api} onSessionEnd={onSessionEnd} />}
          </div>
        )}

        <div className="pointer-events-none absolute inset-x-0 bottom-3 flex justify-center px-3">
          <div className="pointer-events-auto flex items-center gap-1 rounded-full border border-line bg-surface-raised/85 p-1 shadow-raised backdrop-blur-md">
            <IconButton label={t.showDetails} onClick={() => setSheetOpen(true)} className="rounded-full md:hidden">
              <Info {...icon} />
            </IconButton>
            <div role="group" aria-label={t.messageViews} className="flex gap-0.5">
              {views.map((key) => (
                <button
                  key={key}
                  type="button"
                  aria-pressed={key === view}
                  disabled={(key === 'rendered' || key === 'source') && !opened.html}
                  onClick={() => setChosen(key)}
                  className="relative h-8 rounded-full px-3 font-medium text-fg-muted transition-[color,background-color,scale] duration-(--duration-fast) ease-out before:absolute before:inset-x-0 before:-inset-y-1 hover:text-fg active:scale-[0.96] disabled:pointer-events-none disabled:text-fg-disabled aria-pressed:bg-selected aria-pressed:text-fg max-md:h-11 max-md:px-3 max-md:before:hidden"
                >
                  {t[key]}
                </button>
              ))}
            </div>
            <span aria-hidden className="mx-1 h-4 w-px bg-line-strong max-md:hidden" />
            <WidthToggle value={preview.width} onChange={preview.onWidth} disabled={view !== 'rendered'} className="max-md:hidden [&>button]:rounded-full" />
          </div>
        </div>
      </div>

      {preview.inspector && (
        <aside aria-label={t.details} className="w-80 shrink-0 overflow-y-auto border-l border-line bg-surface max-md:hidden">
          <Inspector message={opened} api={api} />
        </aside>
      )}

      <div
        aria-hidden
        data-open={sheetOpen}
        onClick={() => setSheetOpen(false)}
        className="absolute inset-0 z-20 bg-scrim opacity-0 transition-opacity duration-(--duration-base) ease-out data-[open=false]:pointer-events-none data-[open=true]:opacity-100 md:hidden"
      />
      <aside
        aria-label={t.details}
        inert={!sheetOpen}
        data-open={sheetOpen}
        className="absolute inset-x-0 bottom-0 z-30 flex max-h-[84%] translate-y-full flex-col rounded-t-xl border-t border-line bg-surface transition-transform duration-(--duration-base) ease-out data-[open=true]:translate-y-0 data-[open=true]:shadow-sheet data-[open=true]:duration-(--duration-slow) md:hidden"
      >
        <div className="flex shrink-0 items-center justify-between py-1 pr-1 pl-gutter">
          <span className="font-semibold">{t.details}</span>
          <IconButton label={t.closeDetails} onClick={() => setSheetOpen(false)}>
            <X {...icon} />
          </IconButton>
        </div>
        <div className="min-h-0 overflow-y-auto pb-[env(safe-area-inset-bottom)]">
          <Inspector message={opened} api={api} />
        </div>
      </aside>
    </div>
  )
}

function DeleteButton({ onDelete }: { onDelete: () => Promise<unknown> }) {
  const [state, submit] = useSubmission(onDelete, () => {})
  return (
    <form onSubmit={submit} className="relative">
      <IconButton type="submit" label={t.deleteMessage} disabled={state === 'busy'}>
        <Trash2 {...icon} />
      </IconButton>
      {state === 'failed' && (
        <div className="absolute top-full right-0 z-10 mt-1 w-max max-w-64 rounded-md border border-line bg-surface-raised px-3 py-2 text-label shadow-raised">
          <Refusal state={state} />
        </div>
      )}
    </form>
  )
}

const widths = [
  { key: 'mobile', Icon: Smartphone },
  { key: 'tablet', Icon: Tablet },
  { key: 'desktop', Icon: Monitor },
] as const

function WidthToggle({ value, onChange, disabled, className }: { value: Width; onChange: (width: Width) => void; disabled?: boolean; className?: string }) {
  return (
    <div role="radiogroup" aria-label={t.previewWidth} className={cn('inline-flex shrink-0 gap-0.5', className)}>
      {widths.map(({ key, Icon }) => (
        <button
          key={key}
          type="button"
          role="radio"
          aria-checked={key === value}
          aria-label={t[key]}
          title={t[key]}
          disabled={disabled}
          onClick={() => onChange(key)}
          className="relative grid size-8 place-items-center rounded-sm before:absolute before:-inset-1 max-md:before:hidden text-fg-subtle transition-[color,background-color,scale] duration-(--duration-fast) ease-out hover:text-fg active:scale-[0.96] disabled:pointer-events-none disabled:text-fg-disabled aria-checked:bg-selected aria-checked:text-fg disabled:aria-checked:bg-transparent disabled:aria-checked:text-fg-disabled max-md:size-11"
        >
          <Icon {...icon} />
        </button>
      ))}
    </div>
  )
}

const logicalWidth: Record<Width, (available: number) => number> = {
  mobile: (available) => (available <= 480 ? available : 375),
  tablet: () => 768,
  desktop: (available) => (available >= 720 ? available : 1024),
}

function EmailFrame({ src, title, width }: { src: string; title: string; width: Width }) {
  const box = useRef<HTMLDivElement>(null)
  const [available, setAvailable] = useState({ width: 0, height: 0 })

  useLayoutEffect(() => {
    const observer = new ResizeObserver(([entry]) => setAvailable({ width: entry.contentRect.width, height: entry.contentRect.height }))
    observer.observe(box.current!)
    return () => observer.disconnect()
  }, [])

  const logical = logicalWidth[width](available.width)
  const scale = Math.min(1, available.width / logical)
  const height = width === 'mobile' ? Math.min(available.height, 780) : available.height

  return (
    <div className="flex min-h-0 flex-1 px-3 pb-18 md:px-6">
      <div ref={box} className="flex min-w-0 flex-1 overflow-hidden">
        {available.width > 0 && (
          <div
            className="m-auto shrink-0 overflow-hidden rounded-lg bg-email-canvas shadow-raised transition-[width] duration-(--duration-base) ease-in-out md:rounded-xl"
            style={{ width: logical * scale, height }}
          >
            <iframe
              title={title}
              src={src}
              sandbox="allow-popups allow-popups-to-escape-sandbox"
              className="block origin-top-left border-0 scheme-light"
              style={{ width: logical, height: height / scale, transform: `scale(${scale})` }}
            />
          </div>
        )}
      </div>
    </div>
  )
}

function Code({ children }: { children: string }) {
  return <pre className="font-mono text-label wrap-anywhere whitespace-pre-wrap text-fg-muted">{children}</pre>
}

function RawSource({ api, onSessionEnd }: { api: string; onSessionEnd: () => void }) {
  const [raw, setRaw] = useState<string | 'failed'>()

  useEffect(() => {
    request('GET', `${api}/raw`, undefined, onSessionEnd)
      .then((response) => response.text())
      .then(setRaw, () => setRaw('failed'))
  }, [])

  if (raw === 'failed') return <p role="alert">{t.rawFailed}</p>
  return raw === undefined ? null : <Code>{raw}</Code>
}

function Inspector({ message, api }: { message: OpenedMessage; api: string }) {
  return (
    <div className="flex flex-col gap-7 p-5">
      <section>
        <Eyebrow className="mb-2.5">{t.details}</Eyebrow>
        <dl className="grid grid-cols-[auto_minmax(0,1fr)] gap-x-4 gap-y-2.5">
          <dt className="text-fg-muted">{t.from}</dt>
          <dd className="wrap-anywhere">{message.sender}</dd>
          <dt className="text-fg-muted">{t.to}</dt>
          <dd className="wrap-anywhere">
            {message.recipients.map((recipient, n) => (
              <span key={n} className="block">
                {recipient}
              </span>
            ))}
          </dd>
          <dt className="text-fg-muted">{t.received}</dt>
          <dd className="tabular-nums">{fullDate(new Date(message.receivedAt))}</dd>
          <dt className="text-fg-muted">{t.size}</dt>
          <dd className="tabular-nums">{formatSize(message.size)}</dd>
        </dl>
      </section>
      <section>
        <Eyebrow className="mb-2.5">{t.attachments}</Eyebrow>
        {message.attachments.length === 0 ? (
          <p className="text-fg-subtle">{t.noAttachment}</p>
        ) : (
          <ul className="flex flex-col gap-2">
            {message.attachments.map((file, n) => (
              <li key={n} className="flex items-center gap-3 rounded-md border border-line bg-surface py-2 pr-2 pl-2.5">
                <span className="grid size-9 shrink-0 place-items-center rounded-sm bg-surface-sunken text-fg-muted">
                  <FileText {...icon} />
                </span>
                <span className="min-w-0 flex-1">
                  <span className="block truncate font-medium" title={file.name}>
                    {file.name}
                  </span>
                  <span className="block truncate text-label text-fg-muted">
                    {file.type} · {formatSize(file.size)}
                  </span>
                </span>
                <a
                  href={`${api}/attachments/${n}`}
                  download={file.name}
                  aria-label={t.download(file.name)}
                  title={t.download(file.name)}
                  className={iconButton}
                >
                  <Download {...icon} />
                </a>
              </li>
            ))}
          </ul>
        )}
      </section>
      <Fields
        title={t.envelope}
        rows={[{ name: 'MAIL FROM', value: message.envelopeSender }, ...message.envelopeRecipients.map((value) => ({ name: 'RCPT TO', value }))]}
      />
      <Fields title={t.headers} rows={message.headers} />
    </div>
  )
}

function Fields({ title, rows }: { title: string; rows: Array<{ name: string; value: string }> }) {
  return (
    <section>
      <Eyebrow className="mb-2.5">{title}</Eyebrow>
      <dl className="flex flex-col gap-2 font-mono text-label">
        {rows.map(({ name, value }, n) => (
          <div key={n}>
            <dt className="text-fg-muted">{name}</dt>
            <dd className="wrap-anywhere">{value}</dd>
          </div>
        ))}
      </dl>
    </section>
  )
}

function Notice({ children }: { children: ReactNode }) {
  return <div className="flex flex-1 flex-col items-start gap-2 p-gutter md:p-6">{children}</div>
}
