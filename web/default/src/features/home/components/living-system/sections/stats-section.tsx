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

import { Counter } from '@/components/counter'

interface Stat {
  end: number
  labelKey: string
  color: string
}

const STATS: Stat[] = [
  {
    end: 50,
    labelKey: 'upstream services integrated',
    color: 'var(--color-violet)',
  },
  {
    end: 100,
    labelKey: 'model billing support',
    color: 'var(--color-blue)',
  },
  {
    end: 50,
    labelKey: 'compatible API routes',
    color: 'var(--color-cyan)',
  },
  {
    end: 10,
    labelKey: 'scheduling controls',
    color: 'var(--color-emerald)',
  },
]

/**
 * Stats strip — four big gradient numbers in a bordered card, each with
 * a colored top accent driven by a raw palette token. The Counter
 * component (shared from @/components/counter) animates the value when
 * the card scrolls into view.
 */
export function StatsSection() {
  const { t } = useTranslation()

  return (
    <section
      className='mx-auto max-w-[1240px] px-7 py-15'
      style={{ fontFamily: 'var(--font-geist)' }}
    >
      <div className='bg-muted/30 grid grid-cols-2 overflow-hidden rounded-2xl border border-border md:grid-cols-4'>
        {STATS.map((s, i) => (
          <div
            key={s.labelKey}
            className='relative border-border p-7 md:p-9'
            style={{
              '--stat-color': s.color,
              borderRight:
                i < STATS.length - 1
                  ? '1px solid var(--border)'
                  : undefined,
            } as React.CSSProperties}
          >
            <span
              className='absolute left-0 top-0 h-0.5 w-7'
              style={{ background: 'var(--stat-color)' }}
            />
            <div
              className='bg-clip-text text-4xl font-extrabold leading-none tracking-[-0.04em] text-transparent md:text-[44px]'
              style={{
                backgroundImage:
                  'linear-gradient(135deg, var(--stat-color), color-mix(in srgb, var(--stat-color) 50%, white))',
              }}
            >
              <Counter end={s.end} suffix='+' />
            </div>
            <div className='text-muted-foreground mt-2.5 font-mono text-xs leading-snug tracking-[0.04em]'>
              {t(s.labelKey)}
            </div>
          </div>
        ))}
      </div>
    </section>
  )
}
