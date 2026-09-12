import { cn } from '@/lib/utils'

import type { EndpointProtocol } from '../types'

const PROTOCOL_LABELS: Record<EndpointProtocol, string> = {
  openai: 'OpenAI',
  anthropic: 'Anthropic',
  gemini: 'Gemini',
  gateway: 'Gateway',
  ark: 'Ark',
  mj: 'Midjourney',
  suno: 'Suno',
}

/**
 * Protocol badge labeling the upstream API family (OpenAI / Anthropic / Gemini /
 * Gateway). Muted pill style so it sits beside the title without competing
 * with the HTTP method badge.
 */
export function ProtocolBadge({
  protocol,
  className,
}: {
  protocol: EndpointProtocol
  className?: string
}) {
  return (
    <span
      className={cn(
        'bg-muted text-muted-foreground inline-flex items-center rounded-full px-2 py-0.5 text-[11px] font-medium',
        className
      )}
    >
      {PROTOCOL_LABELS[protocol]}
    </span>
  )
}
