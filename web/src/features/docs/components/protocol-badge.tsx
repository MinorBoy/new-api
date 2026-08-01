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

import type { EndpointProtocol } from '../types'

const PROTOCOL_LABELS: Record<EndpointProtocol, string> = {
  openai: 'OpenAI',
  anthropic: 'Anthropic',
  gemini: 'Gemini',
  gateway: 'Gateway',
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
