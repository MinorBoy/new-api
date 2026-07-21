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

const ITEMS = [
  {
    num: '01',
    titleKey: 'Drop-in base URL',
    subKey:
      'OpenAI-compatible. Swap one string, every model reachable.',
  },
  {
    num: '02',
    titleKey: '50+ upstreams',
    subKey: 'Auto-normalized to a single shape your SDK already speaks.',
  },
  {
    num: '03',
    titleKey: 'Pay per token',
    subKey: 'No markup, no tiers, no seats. Settled at provider cost.',
  },
  {
    num: '04',
    titleKey: 'Self-host, free',
    subKey: 'AGPL-3.0. Run it on your own infra, forever.',
  },
] as const

/**
 * TL;DR — the four-line summary that lives directly under the hero.
 * Pitches the whole offering for impatient visitors before they commit
 * to scrolling the rest of the page.
 */
export function TldrSection() {
  const { t } = useTranslation()

  return (
    <section
      id='tldr'
      className='mx-auto max-w-[1240px] px-7 pt-20'
      style={{ fontFamily: 'var(--font-geist)' }}
    >
      <div className='border-border bg-muted/30 grid grid-cols-[auto_1fr] gap-8 rounded-3xl border p-8 md:p-11'>
        <div
          className='text-muted-foreground/70 hidden border-l-2 border-violet py-2 font-mono text-xs tracking-[0.2em] [writing-mode:vertical-rl] md:block'
          style={{ transform: 'rotate(180deg)' }}
        >
          TL;DR
        </div>
        <div className='grid grid-cols-1 gap-6 sm:grid-cols-2 lg:grid-cols-4'>
          {ITEMS.map((item) => (
            <div
              key={item.num}
              className='border-border grid grid-cols-[auto_1fr] gap-3.5 border-b border-dotted pb-4 last:border-b-0'
            >
              <span className='font-mono text-sm font-semibold text-violet'>
                {item.num}
              </span>
              <div>
                <div className='text-base font-medium leading-snug'>
                  {t(item.titleKey)}
                </div>
                <div className='text-muted-foreground mt-1 text-sm leading-snug'>
                  {t(item.subKey)}
                </div>
              </div>
            </div>
          ))}
        </div>
      </div>
    </section>
  )
}
