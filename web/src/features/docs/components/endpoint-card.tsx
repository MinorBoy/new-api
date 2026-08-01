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
import { Link } from '@tanstack/react-router'

import { resolveDocLocale } from '../lib/resolve-doc'
import { useDocLocale } from '../lib/use-doc-locale'
import type { ApiEndpoint } from '../types'
import { MethodBadge } from './method-badge'
import { ProtocolBadge } from './protocol-badge'

/**
 * One endpoint in the reference catalog grid. Shows the HTTP method badge,
 * localized title, protocol tag, path (mono), and a one-line summary.
 * Clicking navigates to the endpoint detail page.
 */
export function EndpointCard({ endpoint }: { endpoint: ApiEndpoint }) {
  const locale = useDocLocale()

  return (
    <Link
      to='/docs/$'
      params={{ _splat: `reference/${endpoint.slug}` }}
      className='border-border/60 hover:border-primary/40 hover:bg-muted/30 group flex flex-col gap-2 rounded-lg border p-4 transition-colors'
    >
      <div className='flex items-center gap-2'>
        <MethodBadge method={endpoint.method} />
        <ProtocolBadge protocol={endpoint.protocol} />
      </div>
      <h3 className='text-foreground text-sm font-semibold'>
        {resolveDocLocale(endpoint.title, locale)}
      </h3>
      <code className='text-muted-foreground font-mono text-xs'>
        {endpoint.path}
      </code>
      <p className='text-muted-foreground line-clamp-2 text-xs'>
        {resolveDocLocale(endpoint.summary, locale)}
      </p>
    </Link>
  )
}
