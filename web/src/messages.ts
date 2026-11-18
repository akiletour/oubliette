import { useEffect, useState } from 'react'

export type Inbox = { id: string; name: string; messageCount: number }

export type Message = { id: string; receivedAt: string; subject: string; preview: string; sender: string; recipients: string[] }

export type OpenedMessage = Message & { text: string; hasHTML: boolean }

export type ShareLink = { id: string; label: string; token: string; createdAt: string; expiresAt: string | null; lastOpenedAt: string | null; expired: boolean }

export const inboxPath = (inboxId: string) => `/inboxes/${inboxId}`

export const messageAt = (base: string, messageId: string) => `${base}/messages/${messageId}`

export class RequestFailed extends Error {
  status: number

  constructor(status: number) {
    super(`request failed with status ${status}`)
    this.status = status
  }
}

export async function request(method: string, path: string, body: unknown, onSessionEnd: () => void) {
  const response = await fetch(path, {
    method,
    headers: body === undefined ? undefined : { 'Content-Type': 'application/json' },
    body: body === undefined ? undefined : JSON.stringify(body),
  })
  if (response.status === 401) onSessionEnd()
  if (!response.ok) throw new RequestFailed(response.status)
  return response
}

export async function getJSON<T>(path: string, onSessionEnd: () => void): Promise<T> {
  return (await request('GET', path, undefined, onSessionEnd)).json()
}

const named = /^(.*) <([^<>]+)>$/

function senderLabel(sender: string) {
  const name = named.exec(sender)?.[1]
  return name && !/[<@]/.test(name) ? name : sender
}

const addressOf = (recipient: string) => named.exec(recipient)?.[2] ?? recipient

export const fromTo = (message: Message) => `${senderLabel(message.sender)} → ${addressOf(message.recipients[0] ?? '')}`

export function useMinuteTick() {
  const [, setTick] = useState(0)
  useEffect(() => {
    const timer = setInterval(() => setTick((tick) => tick + 1), 60_000)
    return () => clearInterval(timer)
  }, [])
}
