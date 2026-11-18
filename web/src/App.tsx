import { useEffect, useState } from 'react'
import { Shell } from './Shell'
import { SignIn } from './SignIn'

export type Session = { email: string }

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
  return session ? <Shell session={session} onSessionChange={refresh} /> : <SignIn onSignedIn={refresh} />
}
