import { type FormEvent, useEffect, useState } from 'react'

type Session = { email: string }

const focusRing =
  'focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-zinc-900 dark:focus-visible:outline-zinc-100'

const field = `h-10 rounded-lg border border-zinc-300 bg-white px-3 text-base font-normal dark:border-zinc-700 dark:bg-zinc-900 ${focusRing}`

const pressable = `rounded-lg text-sm font-medium transition-transform duration-100 ease-out active:scale-[0.97] disabled:opacity-60 motion-reduce:transition-none ${focusRing}`

const failures: Partial<Record<number, string>> = {
  401: 'Wrong email or password.',
  429: 'Too many attempts. Try again in a few minutes.',
}

async function fetchSession(): Promise<Session | null> {
  try {
    const response = await fetch('/api/session')
    return response.ok ? await response.json() : null
  } catch {
    return null
  }
}

export function App() {
  const [session, setSession] = useState<Session | null>()
  const refresh = () => fetchSession().then(setSession)

  useEffect(() => {
    refresh()
  }, [])

  if (session === undefined) return null
  return session ? <Home session={session} onSignedOut={refresh} /> : <SignIn onSignedIn={refresh} />
}

function SignIn({ onSignedIn }: { onSignedIn: () => Promise<void> }) {
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
      else setError(failures[response.status] ?? 'Something went wrong. Try again.')
    } catch {
      setError('The server did not answer. Try again.')
    }
    setSubmitting(false)
  }

  return (
    <main className="flex min-h-dvh items-center justify-center px-6">
      <form onSubmit={submit} className="flex w-full max-w-xs flex-col gap-4">
        <h1 className="mb-2 text-2xl font-semibold tracking-tight">Sign in to Oubliette</h1>
        <label className="flex flex-col gap-1.5 text-sm font-medium">
          Email
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
        <label className="flex flex-col gap-1.5 text-sm font-medium">
          Password
          <input name="password" type="password" autoComplete="current-password" required className={field} />
        </label>
        <p role="alert" className="min-h-5 text-sm text-red-600 dark:text-red-400">
          {error}
        </p>
        <button
          type="submit"
          disabled={submitting}
          className={`h-10 bg-zinc-900 text-white dark:bg-white dark:text-zinc-900 ${pressable}`}
        >
          {submitting ? 'Signing in…' : 'Sign in'}
        </button>
      </form>
    </main>
  )
}

function Home({ session, onSignedOut }: { session: Session; onSignedOut: () => Promise<void> }) {
  const signOut = () => fetch('/api/session', { method: 'DELETE' }).then(onSignedOut, onSignedOut)

  return (
    <div className="flex min-h-dvh flex-col">
      <header className="flex h-14 items-center justify-between border-b border-zinc-200 px-6 dark:border-zinc-800">
        <span className="font-semibold tracking-tight">Oubliette</span>
        <div className="flex items-center gap-4 text-sm">
          <span className="text-zinc-500 dark:text-zinc-400">{session.email}</span>
          <button
            type="button"
            onClick={signOut}
            className={`h-8 border border-zinc-300 px-3 dark:border-zinc-700 ${pressable}`}
          >
            Sign out
          </button>
        </div>
      </header>
      <main className="flex flex-1 items-center justify-center px-6 text-sm text-zinc-500 dark:text-zinc-400">
        Nothing here yet.
      </main>
    </div>
  )
}
