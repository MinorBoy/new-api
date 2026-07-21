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
import { useReducedMotion } from 'motion/react'

import { cn } from '@/lib/utils'

/**
 * LIVE ROUTING — the proprietary visual effect of the Living System
 * composition. Renders a schematic of one client request fanning out
 * from the YSRouter hub to four upstreams, with particles travelling
 * each path on a loop.
 *
 * Reduced motion: particles render as static dots at their path
 * endpoints (no animation); layout is identical so the diagram still
 * communicates the topology.
 *
 * Geometry is tuned for a 980×200 viewBox with preserveAspectRatio off;
 * the parent container clips overflow. Coordinates are inline so the
 * shape stays legible without consulting external constants.
 */

interface UpstreamNode {
  x: number
  y: number
  color: string
  label: string
}

const HUB_X = 490
const HUB_Y = 100
const CLIENT_X = 100
const CLIENT_Y = 100

const UPSTREAMS: UpstreamNode[] = [
  { x: 880, y: 30, color: 'var(--color-emerald)', label: 'OpenAI' },
  { x: 920, y: 80, color: 'var(--color-amber)', label: 'Claude' },
  { x: 900, y: 130, color: 'var(--color-blue)', label: 'Gemini' },
  { x: 860, y: 170, color: 'var(--color-cyan)', label: 'Mistral' },
]

export function RouteCanvas({ className }: { className?: string }) {
  // useReducedMotion from motion/react returns true when the user has
  // requested reduced motion. We render static endpoints in that case.
  const shouldReduce = useReducedMotion()

  return (
    <div
      className={cn(
        'border-border bg-muted/30 relative mt-15 h-[200px] w-full max-w-[980px] overflow-hidden rounded-2xl border',
        className
      )}
    >
      <span className='text-muted-foreground absolute top-3.5 left-4 z-10 font-mono text-[10px] tracking-[0.18em]'>
        LIVE ROUTING
      </span>
      <span className='absolute top-3.5 right-4 z-10 font-mono text-[10px] tracking-[0.1em] text-emerald-500'>
        ● active
      </span>
      <svg
        className='absolute inset-0 h-full w-full'
        viewBox='0 0 980 200'
        preserveAspectRatio='none'
        role='img'
        aria-label='Schematic of a client request routing through YSRouter to four upstream AI providers'
      >
        {/* Connection lines (dashed, animated via CSS) */}
        <g aria-hidden='true'>
          <line
            x1={CLIENT_X}
            y1={CLIENT_Y}
            x2={HUB_X - 32}
            y2={HUB_Y}
            stroke='var(--color-blue)'
            strokeWidth={1.5}
            strokeOpacity={0.6}
            strokeDasharray='4 6'
            className='route-dash'
          />
          {UPSTREAMS.map((u) => (
            <line
              key={`line-${u.label}`}
              x1={HUB_X}
              y1={HUB_Y}
              x2={u.x}
              y2={u.y}
              stroke={u.color}
              strokeWidth={1.5}
              strokeOpacity={0.5}
              strokeDasharray='4 6'
              className='route-dash'
            />
          ))}
        </g>

        {/* Client node (left) */}
        <g>
          <rect
            x={20}
            y={80}
            width={80}
            height={40}
            rx={8}
            fill='var(--background)'
            stroke='var(--color-blue)'
            strokeWidth={1.5}
          />
          <text
            x={60}
            y={104}
            textAnchor='middle'
            fill='var(--muted-foreground)'
            fontSize={10}
            fontFamily='var(--font-geist-mono)'
          >
            client
          </text>
        </g>

        {/* Hub (center) */}
        <g>
          <circle
            cx={HUB_X}
            cy={HUB_Y}
            r={32}
            fill='var(--background)'
            stroke='var(--color-violet)'
            strokeWidth={2}
          />
          <circle cx={HUB_X} cy={HUB_Y} r={6} fill='var(--color-violet)' />
          <text
            x={HUB_X}
            y={150}
            textAnchor='middle'
            fill='var(--muted-foreground)'
            fontSize={10}
            fontFamily='var(--font-geist-mono)'
          >
            YSRouter
          </text>
        </g>

        {/* Upstream nodes (right) */}
        {UPSTREAMS.map((u) => (
          <g key={`node-${u.label}`}>
            <circle
              cx={u.x}
              cy={u.y}
              r={12}
              fill='var(--background)'
              stroke={u.color}
              strokeWidth={1.5}
            />
            <text
              x={u.x}
              y={u.y - 20}
              textAnchor='middle'
              fill='var(--muted-foreground)'
              fontSize={9}
              fontFamily='var(--font-geist-mono)'
            >
              {u.label}
            </text>
          </g>
        ))}

        {/* Particles — animated via SMIL <animateMotion> following each
         * connection line's path. Reduced motion: render as static dots
         * at the path endpoint (the upstream / hub end). */}
        {!shouldReduce && (
          <g aria-hidden='true'>
            <circle r={3} fill='var(--color-blue)'>
              <animateMotion
                dur='1.8s'
                repeatCount='indefinite'
                path={`M ${CLIENT_X},${CLIENT_Y} L ${HUB_X - 32},${HUB_Y}`}
              />
            </circle>
            {UPSTREAMS.map((u, i) => (
              <circle key={`particle-${u.label}`} r={3} fill={u.color}>
                <animateMotion
                  dur={`${2 + i * 0.3}s`}
                  repeatCount='indefinite'
                  path={`M ${HUB_X},${HUB_Y} L ${u.x},${u.y}`}
                />
              </circle>
            ))}
          </g>
        )}
        {shouldReduce && (
          <g aria-hidden='true'>
            <circle cx={HUB_X - 32} cy={HUB_Y} r={3} fill='var(--color-blue)' />
            {UPSTREAMS.map((u) => (
              <circle key={`static-${u.label}`} cx={u.x} cy={u.y} r={3} fill={u.color} />
            ))}
          </g>
        )}

        {/* Inline <style> for the dashed-line drift. Kept here (not in
         * global CSS) because the keyframe is local to this diagram. The
         * class name is stable (not uid-suffixed) since this landing
         * composition renders at most once per page; the SMIL particle
         * motion above already conveys the routing effect even if the
         * dashed drift is overridden by prefers-reduced-motion in
         * styles/index.css. */}
        <style>{`
          .route-dash { animation: route-dash-drift 1.5s linear infinite; }
          @keyframes route-dash-drift { to { stroke-dashoffset: -20; } }
          @media (prefers-reduced-motion: reduce) {
            .route-dash { animation: none; }
          }
        `}</style>
      </svg>
    </div>
  )
}
