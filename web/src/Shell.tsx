import { useEffect, useState } from 'react'
import { ChevronsUpDown, Eraser, Link2, LogOut, Pencil, Trash2 } from 'lucide-react'
import type { Session } from './App'
import { t } from './i18n'
import { DeleteInboxDialog, EmptyInboxDialog, InboxNameDialog, InboxNav, SwitchInboxDialog } from './Inboxes'
import { RequestFailed, getJSON, inboxPath, messageAt, request, type Inbox, type ShareLink } from './messages'
import { link, navigate, usePath } from './router'
import { SharingDialog } from './Sharing'
import { IconButton, ThemeToggle, cn, icon, primaryButton, textLink } from './ui'
import { MessageList, Workspace, type Place } from './Workspace'

const route = /^\/inboxes\/([^/]+)(?:\/messages\/([^/]+))?$/

type InboxDialog = 'create' | 'rename' | 'empty' | 'delete' | 'switch' | 'share'

const placeOf = (inbox: Inbox): Place => ({
  key: inbox.id,
  name: inbox.name,
  messageCount: inbox.messageCount,
  api: `/api/inboxes/${inbox.id}`,
  page: inboxPath(inbox.id),
})

export function Shell({ session, onSessionChange }: { session: Session; onSessionChange: () => Promise<void> }) {
  const path = usePath().replace(/\/+$/, '') || '/'
  const [, inboxId, messageId] = route.exec(path) ?? []
  const [inboxes, setInboxes] = useState<Inbox[]>()
  const [failed, setFailed] = useState(false)
  const [dialog, setDialog] = useState<InboxDialog>()

  const loadInboxes = () =>
    getJSON<{ inboxes: Inbox[] }>('/api/inboxes', onSessionChange).then(
      (body) => {
        setInboxes(body.inboxes)
        setFailed(false)
      },
      () => setFailed(true),
    )

  useEffect(() => {
    loadInboxes()
  }, [inboxId])

  useEffect(() => {
    if (path === '/' && inboxes?.[0]) navigate(inboxPath(inboxes[0].id), 'replace')
  }, [path, inboxes])

  const selected = inboxes?.find((inbox) => inbox.id === inboxId)
  const place = selected && placeOf(selected)
  const signOut = () => fetch('/api/session', { method: 'DELETE' }).then(onSessionChange, onSessionChange)
  const closeDialog = (kind: InboxDialog) => () => setDialog((current) => (current === kind ? undefined : current))

  async function createInbox(name: string) {
    const created: Inbox = await (await request('POST', '/api/inboxes', { name }, onSessionChange)).json()
    await loadInboxes()
    navigate(inboxPath(created.id))
  }

  async function renameInbox(id: string, name: string) {
    await request('PATCH', `/api/inboxes/${id}`, { name }, onSessionChange)
    await loadInboxes()
  }

  async function deleteInbox(id: string) {
    await request('DELETE', `/api/inboxes/${id}`, undefined, onSessionChange)
    setDialog(undefined)
    setInboxes((listed) => listed?.filter((inbox) => inbox.id !== id))
    navigate('/', 'replace')
  }

  async function emptyInbox(id: string) {
    await request('DELETE', `/api/inboxes/${id}/messages`, undefined, onSessionChange)
    navigate(inboxPath(id))
    await loadInboxes()
  }

  async function deleteMessage(inboxId: string, id: string) {
    try {
      await request('DELETE', messageAt(`/api/inboxes/${inboxId}`, id), undefined, onSessionChange)
    } catch (error) {
      if (!(error instanceof RequestFailed && error.status === 404)) throw error
    }
    navigate(inboxPath(inboxId))
    await loadInboxes()
  }

  const sharingOf = (id: string) => ({
    list: async () => (await getJSON<{ shareLinks: ShareLink[] }>(`/api/inboxes/${id}/share-links`, onSessionChange)).shareLinks,
    create: (label: string, expiresAt?: string) => request('POST', `/api/inboxes/${id}/share-links`, { label, expiresAt }, onSessionChange),
    revoke: (link: string) => request('DELETE', `/api/inboxes/${id}/share-links/${link}`, undefined, onSessionChange),
  })

  return (
    <div className="relative flex h-full overflow-hidden">
      <nav aria-label={t.inboxes} className="flex w-rail shrink-0 flex-col border-r border-line max-lg:hidden">
        <InboxNav inboxes={inboxes ?? []} selectedId={selected?.id} onCreate={() => setDialog('create')} />
        <Account session={session} onSignOut={signOut} className="shrink-0 border-t border-line py-1.5 pr-2 pl-gutter" />
      </nav>

      <Workspace
        place={place}
        messageId={messageId}
        onSessionEnd={onSessionChange}
        onDeleteMessage={selected && ((id) => deleteMessage(selected.id, id))}
        footer={
          <>
            <Account session={session} onSignOut={signOut} className="flex-1 lg:hidden" />
            <span className="ml-auto flex shrink-0">
              <ThemeToggle />
            </span>
          </>
        }
      >
        {place && (
          <MessageList
            key={place.key}
            place={place}
            openId={messageId}
            onSessionEnd={onSessionChange}
            onLiveChange={loadInboxes}
            emptyHint={t.emptyInboxHint}
            nameAction={
              <IconButton label={t.switchInbox} onClick={() => setDialog('switch')} className="-ml-1 lg:hidden">
                <ChevronsUpDown {...icon} />
              </IconButton>
            }
            actions={
              <>
                <IconButton label={t.shareInbox} onClick={() => setDialog('share')}>
                  <Link2 {...icon} />
                </IconButton>
                <IconButton label={t.renameInbox} onClick={() => setDialog('rename')}>
                  <Pencil {...icon} />
                </IconButton>
                <IconButton label={t.emptyTheInbox} onClick={() => setDialog('empty')} disabled={place.messageCount === 0}>
                  <Eraser {...icon} />
                </IconButton>
                <IconButton label={t.deleteInbox} onClick={() => setDialog('delete')}>
                  <Trash2 {...icon} />
                </IconButton>
              </>
            }
          />
        )}
        {failed && !inboxes && (
          <p role="alert" className="p-gutter">
            {t.failed}
          </p>
        )}
        {inboxes?.length === 0 && (
          <div className="flex flex-col items-start gap-3 p-gutter">
            <div>
              <p className="font-medium">{t.noInbox}</p>
              <p className="mt-1 text-fg-muted">{t.noInboxHint}</p>
            </div>
            <button type="button" onClick={() => setDialog('create')} className={primaryButton}>
              {t.newInbox}
            </button>
          </div>
        )}
        {inboxes && inboxes.length > 0 && !selected && path !== '/' && (
          <div className="flex flex-col items-start gap-2 p-gutter">
            <p className="font-medium">{t.inboxMissing}</p>
            <a {...link(inboxPath(inboxes[0].id))} className={textLink}>
              {t.goToInbox(inboxes[0].name)}
            </a>
          </div>
        )}
      </Workspace>

      {dialog === 'create' && <InboxNameDialog title={t.newInbox} action={t.create} onSubmit={createInbox} onClose={closeDialog('create')} />}
      {dialog === 'rename' && selected && (
        <InboxNameDialog
          title={t.renameInbox}
          action={t.rename}
          initial={selected.name}
          onSubmit={(name) => renameInbox(selected.id, name)}
          onClose={closeDialog('rename')}
        />
      )}
      {dialog === 'empty' && selected && (
        <EmptyInboxDialog inbox={selected} onEmpty={() => emptyInbox(selected.id)} onClose={closeDialog('empty')} />
      )}
      {dialog === 'delete' && selected && (
        <DeleteInboxDialog inbox={selected} onDelete={() => deleteInbox(selected.id)} onClose={closeDialog('delete')} />
      )}
      {dialog === 'share' && selected && (
        <SharingDialog inbox={selected} sharing={sharingOf(selected.id)} onClose={closeDialog('share')} />
      )}
      {dialog === 'switch' && inboxes && (
        <SwitchInboxDialog inboxes={inboxes} selectedId={selected?.id} onCreate={() => setDialog('create')} onClose={closeDialog('switch')} />
      )}
    </div>
  )
}

function Account({ session, onSignOut, className }: { session: Session; onSignOut: () => void; className?: string }) {
  return (
    <div className={cn('flex min-w-0 items-center gap-2', className)}>
      <span className="truncate text-label text-fg-muted" title={session.email}>
        {session.email}
      </span>
      <IconButton label={t.signOut} onClick={onSignOut} className="ml-auto">
        <LogOut {...icon} />
      </IconButton>
    </div>
  )
}
