import { useEffect, useState } from 'react'
import { relativeTime, shortDate, t } from './i18n'
import type { ShareLink } from './messages'
import { CopyButton, Refusal, borderedButton, cn, field, primaryButton, secondaryButton, useSubmission } from './ui'

export type Sharing = {
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

export function SharingTab({ sharing }: { sharing: Sharing }) {
  const [links, setLinks] = useState<ShareLink[] | 'failed'>()
  const reload = () => sharing.list().then(setLinks, () => setLinks('failed'))

  useEffect(() => {
    reload()
  }, [])

  return (
    <>
      <div className="px-5 py-4">
        <p className="mb-4 text-fg-muted">{t.shareWarning}</p>
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
    </>
  )
}

function LinkRow({ link, onRevoke }: { link: ShareLink; onRevoke: () => Promise<unknown> }) {
  const [confirming, setConfirming] = useState(false)
  const [state, revoke] = useSubmission(onRevoke, () => {})
  const expires = link.expiresAt ? new Date(link.expiresAt) : undefined

  return (
    <li className="flex flex-col gap-2 px-4 py-3">
      <div className="flex items-center gap-2">
        <p className="min-w-0 flex-1 truncate font-medium">{link.label}</p>
        <CopyButton label={t.copyLink(link.label)} text={shareURL(link)} />
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
    <form onSubmit={submit} className="flex flex-col gap-3 border-t border-line px-5 py-4">
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
