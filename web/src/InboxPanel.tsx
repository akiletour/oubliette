import { useEffect, useRef, useState, type KeyboardEvent } from 'react'
import { t } from './i18n'
import type { Credentials, Inbox } from './messages'
import { SharingTab, type Sharing } from './Sharing'
import { CopyButton, Dialog, Refusal, borderedButton, primaryButton, secondaryButton, useSubmission } from './ui'

const tabs = ['integration', 'sharing'] as const

export type PanelTab = (typeof tabs)[number]

export type Integration = { show: () => Promise<Credentials>; regenerate: () => Promise<Credentials> }

export function InboxPanel({
  inbox,
  tab,
  sharing,
  integration,
  onClose,
}: {
  inbox: Inbox
  tab: PanelTab
  sharing: Sharing
  integration: Integration
  onClose: () => void
}) {
  return (
    <Dialog title={inbox.name} wide onClose={onClose}>
      {(close) => <Panel name={inbox.name} initialTab={tab} sharing={sharing} integration={integration} close={close} />}
    </Dialog>
  )
}

function Panel({
  name,
  initialTab,
  sharing,
  integration,
  close,
}: {
  name: string
  initialTab: PanelTab
  sharing: Sharing
  integration: Integration
  close: () => void
}) {
  const [tab, setTab] = useState(initialTab)
  const tabList = useRef<HTMLDivElement>(null)

  function moveWithArrows(event: KeyboardEvent) {
    const step = { ArrowRight: 1, ArrowLeft: -1 }[event.key]
    if (!step) return
    const next = tabs[(tabs.indexOf(tab) + step + tabs.length) % tabs.length]
    setTab(next)
    tabList.current?.querySelector<HTMLElement>(`#panel-tab-${next}`)?.focus()
  }

  return (
    <div className="flex max-h-160 flex-col">
      <div className="shrink-0 px-5 pt-5">
        <h2 className="truncate text-heading font-semibold">{name}</h2>
        <div ref={tabList} role="tablist" aria-label={name} onKeyDown={moveWithArrows} className="mt-3 flex gap-0.5">
          {tabs.map((key) => (
            <button
              key={key}
              id={`panel-tab-${key}`}
              type="button"
              role="tab"
              aria-selected={key === tab}
              aria-controls="panel-content"
              tabIndex={key === tab ? 0 : -1}
              onClick={() => setTab(key)}
              className="relative h-8 rounded-full px-3 font-medium before:absolute before:inset-x-0 before:-inset-y-1 max-md:h-11 max-md:before:hidden text-fg-muted transition-[color,background-color,scale] duration-(--duration-fast) ease-out hover:text-fg active:scale-[0.96] aria-selected:bg-selected aria-selected:text-fg"
            >
              {t[key]}
            </button>
          ))}
        </div>
      </div>
      <div id="panel-content" role="tabpanel" aria-labelledby={`panel-tab-${tab}`} className="min-h-0 flex-1 overflow-y-auto">
        {tab === 'integration' ? <IntegrationTab integration={integration} /> : <SharingTab sharing={sharing} />}
      </div>
      <div className="flex shrink-0 justify-end border-t border-line px-5 py-4">
        <button type="button" onClick={close} className={primaryButton}>
          {t.done}
        </button>
      </div>
    </div>
  )
}


function IntegrationTab({ integration }: { integration: Integration }) {
  const [shown, setShown] = useState<Credentials | 'failed'>()

  useEffect(() => {
    integration.show().then(setShown, () => setShown('failed'))
  }, [])

  if (shown === undefined) return null
  if (shown === 'failed') {
    return (
      <div className="px-5 py-4">
        <Refusal state="failed" />
      </div>
    )
  }

  const host = shown.smtpHost || window.location.hostname
  const values = [
    [t.smtpHost, host],
    [t.smtpPort, String(shown.smtpPort)],
    [t.smtpUsername, shown.smtpUsername],
    [t.password, shown.smtpPassword],
  ]
  const environment = [`SMTP_HOST=${host}`, `SMTP_PORT=${shown.smtpPort}`, `SMTP_USERNAME=${shown.smtpUsername}`, `SMTP_PASSWORD=${shown.smtpPassword}`].join('\n')
  return (
    <div className="flex flex-col gap-5 px-5 py-4">
      <p className="text-fg-muted">{t.integrationHint}</p>
      <dl className="flex flex-col divide-y divide-line rounded-lg border border-line">
        {values.map(([label, value]) => (
          <div key={label} className="flex items-center gap-3 py-1 pr-1 pl-4">
            <dt className="w-28 shrink-0 text-label text-fg-muted">{label}</dt>
            <dd className="min-w-0 flex-1 truncate font-mono">{value}</dd>
            <CopyButton key={value} label={t.copyValue(label)} text={value} />
          </div>
        ))}
      </dl>
      <section className="flex flex-col gap-2">
        <div className="flex items-center justify-between">
          <h3 className="font-semibold">{t.environment}</h3>
          <CopyButton key={environment} label={t.copyEnvironment} text={environment} />
        </div>
        <pre className="overflow-x-auto rounded-lg border border-line bg-surface-sunken px-4 py-3 font-mono text-label">{environment}</pre>
      </section>
      <Regenerate onRegenerate={() => integration.regenerate().then(setShown)} />
    </div>
  )
}

function Regenerate({ onRegenerate }: { onRegenerate: () => Promise<unknown> }) {
  const [confirming, setConfirming] = useState(false)
  const [state, submit] = useSubmission(onRegenerate, () => setConfirming(false))
  return (
    <section className="flex flex-col items-start gap-2 border-t border-line pt-4">
      <h3 className="font-semibold">{t.regenerateTitle}</h3>
      <p className="text-fg-muted">{t.regenerateHint}</p>
      {confirming ? (
        <form onSubmit={submit} className="flex w-full flex-wrap items-center gap-2 rounded-md bg-surface-sunken px-3 py-2">
          <p className="min-w-0 flex-1 text-label">{t.regenerateConfirm}</p>
          <button type="button" onClick={() => setConfirming(false)} className={secondaryButton}>
            {t.cancel}
          </button>
          <button type="submit" disabled={state === 'busy'} className={primaryButton}>
            {t.regenerate}
          </button>
          <Refusal state={state} />
        </form>
      ) : (
        <button type="button" onClick={() => setConfirming(true)} className={borderedButton}>
          {t.regenerate}
        </button>
      )}
    </section>
  )
}
