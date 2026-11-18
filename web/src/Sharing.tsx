import { useEffect, useState } from 'react'
import { Check, Copy } from 'lucide-react'
import { relativeTime, shortDate, t } from './i18n'
import type { Inbox, ShareLink } from './messages'
import { Dialog, IconButton, Refusal, borderedButton, cn, field, icon, primaryButton, secondaryButton, useSubmission } from './ui'

type Sharing = {
  list: () => Promise<ShareLink[]>
  create: (label: string, expiresAt?: string) => Promise<unknown>
  revoke: (id: string) => Promise<unknown>
}

const shareURL = (link: ShareLink) => `${window.location.origin}/s/${link.token}`

function today() {
  const now = new Date()
  return `${now.getFullYear()}-${String(now.getMonth() + 1).padStart(2, '0')}-${String(now.getDate()).padStart(2, '0')}`
}

function endOfDay(day: string) {
  const [year, month, date] = day.split('-').map(Number)
  return new Date(year, month - 1, date, 23, 59, 59).toISOString()
}

export function SharingDialog({ inbox, sharing, onClose }: { inbox: Inbox; sharing: Sharing; onClose: () => void }) {
  const title = t.shareTitle(inbox.name)
  return (
    <Dialog title={title} wide onClose={onClose}>
      {(close) => <SharingPanel title={title} sharing={sharing} close={close} />}
    </Dialog>
  )
}

function SharingPanel({ title, sharing, close }: { title: string; sharing: Sharing; close: () => void }) {
  const [links, setLinks] = useState<ShareLink[] | 'failed'>()
  const reload = () => sharing.list().then(setLinks, () => setLinks('failed'))

  useEffect(() => {
    reload()
  }, [])

  return (
    <div className="flex max-h-160 flex-col">
      <div className="shrink-0 px-5 pt-5">
        <h2 className="text-heading font-semibold text-balance">{title}</h2>
        <p className="mt-1 text-fg-muted">{t.shareWarning}</p>
      </div>
      <div className="min-h-0 flex-1 overflow-y-auto px-5 py-4">
        {links === 'failed' && <Refusal state="failed" />}
        {links?.length === 0 && <p className="text-fg-muted">{t.noShareLink}</p>}
        {Array.isArray(links) && links.length > 0 && (
          <ul className="flex flex-col divide-y divide-line rounded-lg border border-line">
            {links.map((link) => (
              <LinkRow key={link.id} link={link} onRevoke={() => sharing.revoke(link.id).then(reload)} />
            ))}
          </ul>
        )}
      </div>
      <NewLink create={(label, expiresAt) => sharing.create(label, expiresAt).then(reload)} />
      <div className="flex shrink-0 justify-end border-t border-line px-5 py-4">
        <button type="button" onClick={close} className={primaryButton}>
          {t.done}
        </button>
      </div>
    </div>
  )
}

function LinkRow({ link, onRevoke }: { link: ShareLink; onRevoke: () => Promise<unknown> }) {
  const [copied, setCopied] = useState(false)
  const [confirming, setConfirming] = useState(false)
  const [state, revoke] = useSubmission(onRevoke, () => {})
  const expires = link.expiresAt ? new Date(link.expiresAt) : undefined
  const copy = () => navigator.clipboard?.writeText(shareURL(link)).then(() => setCopied(true))

  return (
    <li className="flex flex-col gap-2 px-4 py-3">
      <div className="flex items-center gap-2">
        <p className="min-w-0 flex-1 truncate font-medium">{link.label}</p>
        <IconButton label={t.copyLink(link.label)} onClick={copy}>
          {copied ? <Check {...icon} /> : <Copy {...icon} />}
        </IconButton>
        {!confirming && (
          <button type="button" onClick={() => setConfirming(true)} aria-label={t.revokeLink(link.label)} className={secondaryButton}>
            {t.revoke}
          </button>
        )}
      </div>
      <p className="text-label text-fg-muted tabular-nums">
        {[
          t.createdOn(shortDate(new Date(link.createdAt))),
          expires ? (link.expired ? t.expiredOn : t.expiresOn)(shortDate(expires)) : t.noExpiry,
          link.lastOpenedAt ? t.openedAgo(relativeTime(new Date(link.lastOpenedAt))) : t.neverOpened,
        ].join(' · ')}
      </p>
      <input
        value={shareURL(link)}
        readOnly
        aria-label={t.linkOf(link.label)}
        onFocus={(event) => event.currentTarget.select()}
        className={cn(field, 'w-full font-mono text-fg-muted')}
      />
      {confirming && (
        <form onSubmit={revoke} className="flex flex-wrap items-center gap-2 rounded-md bg-surface-sunken px-3 py-2">
          <p className="min-w-0 flex-1 text-label">{t.revokeConfirm}</p>
          <button type="button" onClick={() => setConfirming(false)} className={secondaryButton}>
            {t.cancel}
          </button>
          <button type="submit" disabled={state === 'busy'} className={primaryButton}>
            {t.revoke}
          </button>
          <Refusal state={state} />
        </form>
      )}
    </li>
  )
}

function NewLink({ create }: { create: (label: string, expiresAt?: string) => Promise<unknown> }) {
  const [label, setLabel] = useState('')
  const [day, setDay] = useState('')
  const [state, submit] = useSubmission(
    () => create(label, day ? endOfDay(day) : undefined),
    () => {
      setLabel('')
      setDay('')
    },
  )
  return (
    <form onSubmit={submit} className="flex shrink-0 flex-col gap-3 border-t border-line px-5 py-4">
      <h3 className="font-semibold">{t.newShareLink}</h3>
      <div className="flex flex-col gap-3 md:flex-row md:items-end">
        <label className="flex min-w-0 flex-1 flex-col gap-1.5 text-label font-medium text-fg-muted">
          {t.shareLabel}
          <input
            value={label}
            onChange={(event) => setLabel(event.target.value)}
            placeholder={t.shareLabelExample}
            required
            spellCheck={false}
            autoComplete="off"
            className={field}
          />
        </label>
        <label className="flex flex-col gap-1.5 text-label font-medium text-fg-muted">
          {t.expiry}
          <input type="date" value={day} min={today()} onChange={(event) => setDay(event.target.value)} className={field} />
        </label>
        <button type="submit" disabled={state === 'busy' || !label.trim()} className={borderedButton}>
          {t.createLink}
        </button>
      </div>
      <Refusal state={state} />
    </form>
  )
}
