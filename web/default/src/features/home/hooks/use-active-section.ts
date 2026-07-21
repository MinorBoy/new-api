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
import { useEffect, useState } from 'react'

/**
 * Track which section id is currently "active" based on scroll position.
 *
 * Uses IntersectionObserver with a rootMargin band centered on the
 * viewport middle (top -40%, bottom -55%) so the active section is the
 * one whose heading sits roughly at eye level — not the one whose body
 * fills the viewport bottom. Returns the first id when nothing has
 * scrolled into the band yet.
 *
 * Reduced motion: skips observer setup entirely and pins to the first
 * id (the rail still renders but does not highlight on scroll — the
 * user can still click to jump).
 */
export function useActiveSection(ids: string[]): string {
  const [active, setActive] = useState(ids[0] ?? '')

  useEffect(() => {
    if (ids.length === 0) return

    const mq = window.matchMedia('(prefers-reduced-motion: reduce)')
    if (mq.matches) {
      setActive(ids[0] ?? '')
      return
    }

    const observer = new IntersectionObserver(
      (entries) => {
        // Pick the most-recently intersecting entry near the top of the band.
        const visible = entries
          .filter((e) => e.isIntersecting)
          .sort((a, b) => a.boundingClientRect.top - b.boundingClientRect.top)
        if (visible[0]) {
          setActive(visible[0].target.id)
        }
      },
      { rootMargin: '-40% 0px -55% 0px', threshold: 0 }
    )

    const observed: Element[] = []
    for (const id of ids) {
      // Follow the project's existing getElementById convention
      // (see turnstile.tsx, send-to-fluent.ts); the unicorn
      // prefer-query-selector rule is a baseline warning here.
      const el = document.getElementById(id)
      if (el) {
        observer.observe(el)
        observed.push(el)
      }
    }

    return () => {
      for (const el of observed) observer.unobserve(el)
      observer.disconnect()
    }
  }, [ids])

  return active
}
