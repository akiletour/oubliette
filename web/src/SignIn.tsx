import { type FormEvent, useState } from 'react'
import { CircleAlert } from 'lucide-react'
import { t } from './i18n'
import { Mark, field, icon, primaryButton } from './ui'

const refusals: Partial<Record<number, string>> = {
  401: t.signInRefused,
  429: t.signInSlowedDown,
}

export function SignIn({ onSignedIn }: { onSignedIn: () => Promise<void> }) {
  const [error, setError] = useState('')
  const [submitting, setSubmitting] = useState(false)

  async function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault()
    const fields = new FormData(event.currentTarget)
    setSubmitting(true)
    setError('')
    try {
      const response = await fetch('/api/session', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ email: fields.get('email'), password: fields.get('password') }),
      })
      if (response.ok) await onSignedIn()
      else setError(refusals[response.status] ?? t.failed)
    } catch {
      setError(t.unreachable)
    }
    setSubmitting(false)
  }

  return (
    <main className="grid min-h-full place-items-center px-gutter py-10">
      <form onSubmit={submit} className="flex w-full max-w-80 flex-col gap-4">
        <Mark className="size-7" />
        <h1 className="mb-2 text-display font-semibold text-balance">{t.signInTitle}</h1>
        <label className="flex flex-col gap-1.5 text-label font-medium text-fg-muted">
          {t.email}
          <input
            name="email"
            type="email"
            autoComplete="username"
            spellCheck={false}
            autoFocus={!('ontouchstart' in window)}
            required
            className={field}
          />
        </label>
        <label className="flex flex-col gap-1.5 text-label font-medium text-fg-muted">
          {t.password}
          <input name="password" type="password" autoComplete="current-password" required className={field} />
        </label>
        <p role="alert" className="flex min-h-5 items-center gap-2">
          {error && (
            <>
              <CircleAlert {...icon} className="shrink-0" />
              {error}
            </>
          )}
        </p>
        <button type="submit" disabled={submitting} className={primaryButton}>
          {submitting ? t.signingIn : t.signIn}
        </button>
      </form>
    </main>
  )
}
