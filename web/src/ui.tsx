import { useEffect, useRef, useState, type ButtonHTMLAttributes, type FormEvent, type ReactNode } from 'react'
import { CircleAlert, Moon, Search, Sun, X } from 'lucide-react'
import { t } from './i18n'
import { RequestFailed } from './messages'
import { setTheme, useTheme } from './theme'

export const cn = (...parts: Array<string | false | null | undefined>) => parts.filter(Boolean).join(' ')

export const field =
  'h-11 rounded-md border border-line-strong bg-surface px-3 text-input font-normal text-fg transition-[border-color] duration-(--duration-fast) ease-out hover:border-fg-subtle md:h-10 md:text-body'

const button = 'h-11 rounded-md px-4 font-medium transition-[scale,color,background-color] duration-(--duration-fast) ease-out active:scale-[0.96] md:h-10'

export const primaryButton = cn(button, 'bg-fg text-bg disabled:bg-fg-disabled')

export const secondaryButton = cn(button, 'text-fg-muted hover:bg-hover hover:text-fg')

export const borderedButton = cn(secondaryButton, 'border border-line-strong')

export const icon = { size: 16, strokeWidth: 1.75, 'aria-hidden': true } as const

export const pressable =
  'rounded-md text-fg-muted transition-[color,background-color,scale] duration-(--duration-fast) ease-out hover:bg-hover hover:text-fg active:scale-[0.96] disabled:pointer-events-none disabled:text-fg-disabled'

export const textLink = 'font-medium text-accent underline-offset-4 hover:underline max-md:py-3'

export function Mark({ decorative, className }: { decorative?: boolean; className?: string }) {
  return (
    <svg
      viewBox="0 0 20 20"
      role={decorative ? undefined : 'img'}
      aria-label={decorative ? undefined : 'Oubliette'}
      aria-hidden={decorative}
      className={cn('size-5 shrink-0', className)}
    >
      <circle cx="10" cy="10" r="8.25" fill="none" stroke="currentColor" strokeWidth="1.5" />
      <circle cx="10" cy="12" r="3.25" fill="currentColor" />
    </svg>
  )
}

export function IconButton({
  label,
  className,
  children,
  ...props
}: { label: string } & ButtonHTMLAttributes<HTMLButtonElement>) {
  return (
    <button
      type="button"
      aria-label={label}
      title={label}
      className={cn(
        pressable,
        'relative grid size-8 shrink-0 place-items-center before:absolute before:-inset-1 max-md:size-11 max-md:before:hidden',
        className,
      )}
      {...props}
    >
      {children}
    </button>
  )
}

export function Dialog({
  title,
  wide,
  onClose,
  children,
}: {
  title: string
  wide?: boolean
  onClose: () => void
  children: (close: () => void) => ReactNode
}) {
  const dialog = useRef<HTMLDialogElement>(null)
  useEffect(() => dialog.current?.showModal(), [])
  return (
    <dialog
      ref={dialog}
      aria-label={title}
      onClose={onClose}
      onClick={(event) => event.target === event.currentTarget && event.currentTarget.close()}
      className={cn(
        'mx-gutter my-auto max-w-none rounded-xl border border-line bg-surface-raised text-fg shadow-sheet transition-[opacity,translate] duration-(--duration-slow) ease-out backdrop:bg-scrim starting:translate-y-2 starting:opacity-0 md:mx-auto',
        wide ? 'md:w-xl' : 'md:w-96',
      )}
    >
      {children(() => dialog.current?.close())}
    </dialog>
  )
}

export function FilterField({ label, value, onChange, className }: { label: string; value: string; onChange: (value: string) => void; className?: string }) {
  return (
    <div className={cn('relative flex items-center', className)}>
      <Search {...icon} className="pointer-events-none absolute left-2.5 text-fg-subtle" />
      <input
        type="search"
        value={value}
        onChange={(event) => onChange(event.target.value)}
        aria-label={label}
        placeholder={label}
        spellCheck={false}
        autoComplete="off"
        className="h-11 w-full rounded-md border border-line-strong bg-surface pr-9 pl-8 text-input text-fg transition-[border-color] duration-(--duration-fast) ease-out placeholder:text-fg-subtle hover:border-fg-subtle md:h-8 md:text-body [&::-webkit-search-cancel-button]:hidden"
      />
      {value && (
        <button
          type="button"
          aria-label={t.clearFilter}
          onClick={() => onChange('')}
          className="absolute right-0 grid size-11 place-items-center rounded-md text-fg-subtle transition-[color,scale] duration-(--duration-fast) ease-out before:absolute before:-inset-1 hover:text-fg active:scale-[0.96] max-md:before:hidden md:size-8"
        >
          <X {...icon} />
        </button>
      )}
    </div>
  )
}

export function Eyebrow({ children, className }: { children: ReactNode; className?: string }) {
  return <h2 className={cn('text-caption font-medium tracking-wider text-fg-subtle uppercase', className)}>{children}</h2>
}

export function ThemeToggle() {
  const theme = useTheme()
  const next = theme === 'dark' ? 'light' : 'dark'
  return (
    <IconButton label={next === 'dark' ? t.toDarkTheme : t.toLightTheme} onClick={() => setTheme(next)}>
      {theme === 'dark' ? <Sun {...icon} /> : <Moon {...icon} />}
    </IconButton>
  )
}

export function Refusal({ state }: { state: Submission }) {
  if (state !== 'failed' && state !== 'refused') return null
  return (
    <p role="alert" className="flex items-start gap-2">
      <CircleAlert {...icon} className="mt-0.5 shrink-0" />
      {state === 'refused' ? t.inboxNameRefused : t.failed}
    </p>
  )
}

type Submission = 'idle' | 'busy' | 'failed' | 'refused'

export function useSubmission<T>(action: () => Promise<T>, done: (result: T) => void) {
  const [state, setState] = useState<Submission>('idle')
  return [
    state,
    async (event: FormEvent) => {
      event.preventDefault()
      setState('busy')
      try {
        done(await action())
        setState('idle')
      } catch (error) {
        setState(error instanceof RequestFailed && error.status === 400 ? 'refused' : 'failed')
      }
    },
  ] as const
}
