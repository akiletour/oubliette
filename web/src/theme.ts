import { useSyncExternalStore } from 'react'

export type Theme = 'light' | 'dark'

const system = window.matchMedia('(prefers-color-scheme: dark)')
const listeners = new Set<() => void>()

const systemTheme = (): Theme => (system.matches ? 'dark' : 'light')

function remembered(): Theme | null {
  try {
    const theme = localStorage.getItem('theme')
    return theme === 'light' || theme === 'dark' ? theme : null
  } catch {
    return null
  }
}

let chosen = remembered()

const currentTheme = () => chosen ?? systemTheme()

function apply() {
  const root = document.documentElement
  root.dataset.switching = ''
  root.dataset.theme = currentTheme()
  listeners.forEach((notify) => notify())
  requestAnimationFrame(() => requestAnimationFrame(() => delete root.dataset.switching))
}

system.addEventListener('change', apply)

window.addEventListener('storage', () => {
  chosen = remembered()
  apply()
})

export function setTheme(theme: Theme) {
  chosen = theme
  try {
    localStorage.setItem('theme', theme)
  } catch {}
  apply()
}

function subscribe(notify: () => void) {
  listeners.add(notify)
  return () => listeners.delete(notify)
}

export function useTheme() {
  return useSyncExternalStore(subscribe, currentTheme)
}
