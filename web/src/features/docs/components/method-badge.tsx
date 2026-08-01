/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program. If not, see <https://www.gnu.org/licenses/>.

For commercial licensing, please contact support@quantumnous.com
*/
import { cn } from '@/lib/utils'

import type { HttpMethod } from '../types'

const METHOD_STYLES: Record<HttpMethod, string> = {
  GET: 'bg-emerald-500/15 text-emerald-600 dark:text-emerald-400 ring-emerald-500/30',
  POST: 'bg-blue-500/15 text-blue-600 dark:text-blue-400 ring-blue-500/30',
  DELETE: 'bg-rose-500/15 text-rose-600 dark:text-rose-400 ring-rose-500/30',
  PUT: 'bg-amber-500/15 text-amber-600 dark:text-amber-400 ring-amber-500/30',
  PATCH: 'bg-violet-500/15 text-violet-600 dark:text-violet-400 ring-violet-500/30',
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
