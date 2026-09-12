import { cn } from '@/lib/utils'

import type { HttpMethod } from '../types'

const METHOD_STYLES: Record<HttpMethod, string> = {
  GET: 'bg-emerald-500/15 text-emerald-600 dark:text-emerald-400 ring-emerald-500/30',
  POST: 'bg-blue-500/15 text-blue-600 dark:text-blue-400 ring-blue-500/30',
  DELETE: 'bg-rose-500/15 text-rose-600 dark:text-rose-400 ring-rose-500/30',
  PUT: 'bg-amber-500/15 text-amber-600 dark:text-amber-400 ring-amber-500/30',
  PATCH:
    'bg-violet-500/15 text-violet-600 dark:text-violet-400 ring-violet-500/30',
}

/**
 * Colored HTTP method badge (4stoken-style). GET → green, POST → blue,
 * DELETE → red. Renders as a fixed-width monospaced chip.
 */
export function MethodBadge({
  method,
  className,
}: {
  method: HttpMethod
  className?: string
}) {
  return (
    <span
      className={cn(
        'inline-flex h-5 min-w-14 items-center justify-center rounded px-1.5 font-mono text-[11px] font-bold tracking-wide ring-1 ring-inset',
        METHOD_STYLES[method],
        className
      )}
    >
      {method}
    </span>
  )
}
