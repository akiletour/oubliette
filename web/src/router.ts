import { useSyncExternalStore, type MouseEvent } from 'react'

const listeners = new Set<() => void>()

function subscribe(notify: () => void) {
  listeners.add(notify)
  window.addEventListener('popstate', notify)
  return () => {
    listeners.delete(notify)
    window.removeEventListener('popstate', notify)
  }
}

export function usePath() {
  return useSyncExternalStore(subscribe, () => window.location.pathname)
}

export function navigate(path: string, mode: 'push' | 'replace' = 'push') {
  if (path === window.location.pathname) return
  window.history[mode === 'push' ? 'pushState' : 'replaceState'](null, '', path)
  listeners.forEach((notify) => notify())
}

export function link(path: string) {
  return {
    href: path,
    onClick: (event: MouseEvent) => {
      if (event.button !== 0 || event.metaKey || event.ctrlKey || event.shiftKey || event.altKey) return
      event.preventDefault()
      navigate(path)
    },
  }
}
