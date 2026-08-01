import { createFileRoute } from '@tanstack/react-router'

import { Docs } from '@/features/docs'

/**
 * Catch-all documentation route. Matches `/docs`, `/docs/quickstart`,
 * `/docs/clients/curl`, etc. The splat (everything after `/docs/`) is forwarded
 * to the `<Docs>` component, which resolves it against the docs manifest.
 */
export const Route = createFileRoute('/docs/$')({
  component: DocsRouteComponent,
})

function DocsRouteComponent() {
  const { _splat } = Route.useParams()
  return <Docs splat={_splat} />
}
