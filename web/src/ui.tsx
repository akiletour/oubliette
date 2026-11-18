import type { ButtonHTMLAttributes, ReactNode } from 'react'

export const cn = (...parts: Array<string | false | null | undefined>) => parts.filter(Boolean).join(' ')

export const icon = { size: 16, strokeWidth: 1.75, 'aria-hidden': true } as const

export const pressable =
  'rounded-md text-fg-muted transition-[color,background-color,scale] duration-(--duration-fast) ease-out hover:bg-hover hover:text-fg active:scale-[0.96] disabled:pointer-events-none disabled:text-fg-disabled'

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

export function Eyebrow({ children, className }: { children: ReactNode; className?: string }) {
  return <h2 className={cn('text-caption font-medium tracking-wider text-fg-subtle uppercase', className)}>{children}</h2>
}
