import { useEffect, useState } from 'react'
import { t } from './i18n'
import { RequestFailed, getJSON } from './messages'
import { usePath } from './router'
import { Mark, ThemeToggle } from './ui'
import { MessageList, Workspace, type Place } from './Workspace'

const route = /^\/s\/([^/]+)(?:\/messages\/([^/]+))?/

const ignore = () => {}

export function Visitor() {
  const [, token = '', messageId] = route.exec(usePath()) ?? []
  const [shared, setShared] = useState<Place | 'gone' | 'failed'>()

  useEffect(() => {
    getJSON<{ name: string; messageCount: number }>(`/api/share/${token}`, ignore).then(
      (inbox) => setShared({ key: token, name: inbox.name, messageCount: inbox.messageCount, api: `/api/share/${token}`, page: `/s/${token}` }),
      (error) => setShared(error instanceof RequestFailed && error.status === 404 ? 'gone' : 'failed'),
    )
  }, [token, messageId])

  if (shared === undefined) return null
  if (shared === 'gone' || shared === 'failed') {
    return (
      <main className="grid min-h-full place-items-center px-gutter py-10">
        <div role={shared === 'failed' ? 'alert' : undefined} className="flex w-full max-w-80 flex-col gap-3">
          <Mark className="size-7" />
          <h1 className="text-heading font-semibold text-balance">{shared === 'gone' ? t.linkGone : t.failed}</h1>
          {shared === 'gone' && <p className="text-fg-muted">{t.linkGoneHint}</p>}
        </div>
      </main>
    )
  }

  return (
    <div className="relative flex h-full overflow-hidden">
      <Workspace
        place={shared}
        messageId={messageId}
        onSessionEnd={ignore}
        inspectorWhenRoomy
        footer={
          <span className="ml-auto flex shrink-0">
            <ThemeToggle />
          </span>
        }
      >
        <MessageList place={shared} openId={messageId} onSessionEnd={ignore} note={t.readOnly} emptyHint={t.emptyInboxVisitorHint} />
      </Workspace>
    </div>
  )
}
