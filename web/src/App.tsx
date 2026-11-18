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
      <main className="flex flex-1 flex-col gap-10 px-6 py-8">
        <Inboxes />
      </main>
    </div>
  )
}

type Inbox = { id: string; name: string }

type Message = { id: string; receivedAt: string; subject: string; sender: string; recipients: string[] }

function Inboxes() {
  const [inboxes, setInboxes] = useState<Inbox[]>([])

  useEffect(() => {
    fetch('/api/inboxes')
      .then((response) => (response.ok ? response.json() : { inboxes: [] }))
      .then((body: { inboxes: Inbox[] }) => setInboxes(body.inboxes))
      .catch(() => setInboxes([]))
  }, [])

  return inboxes.map((inbox) => (
    <section key={inbox.id}>
      <h1 className="mb-4 text-lg font-semibold tracking-tight">{inbox.name}</h1>
      <Messages inbox={inbox} />
    </section>
  ))
}

function Messages({ inbox }: { inbox: Inbox }) {
  const [messages, setMessages] = useState<Message[]>([])
  const [next, setNext] = useState('')
  const [status, setStatus] = useState<'loading' | 'ready' | 'failed'>('loading')

  async function load(before: string) {
    setStatus('loading')
    try {
      const response = await fetch(`/api/inboxes/${inbox.id}/messages?before=${encodeURIComponent(before)}`)
      if (!response.ok) throw new Error(response.statusText)
      const page: { messages: Message[]; next: string } = await response.json()
      setMessages((held) => (before ? [...held, ...page.messages] : page.messages))
      setNext(page.next)
      setStatus('ready')
    } catch {
      setStatus('failed')
    }
  }

  useEffect(() => {
    load('')
  }, [inbox.id])

  if (status === 'failed') {
    return (
      <p role="alert" className="text-sm text-red-600 dark:text-red-400">
        The Messages could not be loaded.
      </p>
    )
  }
  if (messages.length === 0) {
    return status === 'ready' ? <p className="text-sm text-zinc-500 dark:text-zinc-400">No Message yet.</p> : null
  }

  const cell = 'truncate border-b border-zinc-200 py-2 pr-4 dark:border-zinc-800'
  return (
    <>
      <table className="w-full table-fixed text-left text-sm">
        <thead className="text-zinc-500 dark:text-zinc-400">
          <tr>
            <th className={`${cell} w-2/5 font-medium`}>Subject</th>
            <th className={`${cell} font-medium`}>From</th>
            <th className={`${cell} font-medium`}>To</th>
            <th className={`${cell} w-48 font-medium`}>Received</th>
          </tr>
        </thead>
        <tbody>
          {messages.map((message) => (
            <tr key={message.id}>
              <td className={cell}>{message.subject || <span className="text-zinc-500">(no subject)</span>}</td>
              <td className={cell}>{message.sender}</td>
              <td className={cell}>{message.recipients.join(', ')}</td>
              <td className={`${cell} tabular-nums`}>{new Date(message.receivedAt).toLocaleString()}</td>
            </tr>
          ))}
        </tbody>
      </table>
      {next && (
        <button
          type="button"
          onClick={() => load(next)}
          disabled={status === 'loading'}
          className={`mt-4 h-8 border border-zinc-300 px-3 dark:border-zinc-700 ${pressable}`}
        >
          Show older Messages
        </button>
      )}
    </>
  )
}
