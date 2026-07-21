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
import { useTranslation } from 'react-i18next'

import { cn } from '@/lib/utils'

import { useActiveSection } from '../../hooks/use-active-section'

interface ScrollRailSegment {
  id: string
  labelKey: string
}

/**
 * Fixed left-side vertical rail of section segments (guided scrolling).
 * Highlights the currently active section (via useActiveSection) and
 * jumps to it on click. Hidden below the `lg` breakpoint — mobile users
 * get the linear document flow without the rail.
 *
 * The rail fades in only after the user has scrolled past 200px, so the
 * hero gets the full first-paint attention.
 */
export function ScrollRail({
  segments,
}: {
  segments: readonly ScrollRailSegment[]
}) {
  const { t } = useTranslation()
  const ids = segments.map((s) => s.id)
  const active = useActiveSection(ids)

  return (
    <div
      className='scroll-rail pointer-events-none fixed top-1/2 left-6 z-40 hidden -translate-y-1/2 flex-col gap-3.5 opacity-0 transition-opacity duration-300 lg:flex [.scroll-rail-visible_&]:pointer-events-auto [.scroll-rail-visible_&]:opacity-100'
      aria-hidden='true'
    >
      {segments.map((seg) => {
        const isActive = seg.id === active
        return (
          <a
            key={seg.id}
            href={`#${seg.id}`}
            className={cn(
              'group flex cursor-pointer items-center gap-2.5',
              isActive ? 'is-active' : ''
            )}
            onClick={(e) => {
              e.preventDefault()
              document
                .getElementById(seg.id)
                ?.scrollIntoView({ behavior: 'smooth' })
            }}
          >
            <span
              className={cn(
                'h-[18px] w-[3px] rounded-[2px] bg-border transition-all duration-200',
                isActive && 'h-[30px] bg-gradient-to-br from-violet via-blue to-cyan'
              )}
            />
            <span
              className={cn(
                'whitespace-nowrap font-mono text-[10px] tracking-[0.1em] uppercase text-muted-foreground/0 transition-opacity duration-200 group-hover:opacity-100',
                isActive && 'opacity-100'
              )}
            >
              {t(seg.labelKey)}
            </span>
          </a>
        )
      })}
    </div>
  )
}
