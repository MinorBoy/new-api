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
