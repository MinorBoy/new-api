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

interface Provider {
  glyph: string
  name: string
  color: string
}

const PROVIDERS: Provider[] = [
  { glyph: 'O', name: 'OpenAI', color: 'var(--color-emerald)' },
  { glyph: 'A', name: 'Anthropic', color: 'var(--color-amber)' },
  { glyph: 'G', name: 'Gemini', color: 'var(--color-blue)' },
  { glyph: 'M', name: 'Mistral', color: 'var(--color-cyan)' },
  { glyph: 'D', name: 'DeepSeek', color: 'var(--color-violet)' },
  { glyph: 'X', name: 'xAI', color: 'var(--color-rose)' },
  { glyph: 'C', name: 'Cohere', color: 'var(--color-emerald)' },
  { glyph: 'K', name: 'Moonshot', color: 'var(--color-amber)' },
  { glyph: 'Q', name: 'Qwen', color: 'var(--color-blue)' },
  { glyph: 'Z', name: 'Zhipu', color: 'var(--color-cyan)' },
  { glyph: 'D', name: 'Doubao', color: 'var(--color-violet)' },
  { glyph: 'Y', name: 'Yi', color: 'var(--color-rose)' },
  { glyph: 'B', name: 'Baichuan', color: 'var(--color-emerald)' },
  { glyph: 'M', name: 'Minimax', color: 'var(--color-amber)' },
  { glyph: 'S', name: 'Spark', color: 'var(--color-blue)' },
  { glyph: 'A', name: 'Azure', color: 'var(--color-cyan)' },
  { glyph: 'B', name: 'Bedrock', color: 'var(--color-violet)' },
]

/**
 * Model wall — the infinite-canvas / node-graph view of upstreams.
 * Each cell is a circular node with an italic serif glyph (Instrument
 * Serif) tinted by a raw palette color, and a small green status dot
 * to read as "online". The last cell is a dashed "+30 more" affordance.
 */
export function ModelsSection() {
  const { t } = useTranslation()

  return (
    <section
      id='models'
      className='mx-auto max-w-[1240px] px-7 py-25'
      style={{ fontFamily: 'var(--font-geist)' }}
    >
      <div className='mx-auto mb-16 max-w-[680px] text-center'>
        <span className='inline-flex items-center gap-2 rounded-full border border-rose/30 bg-rose/10 px-2.5 py-1 font-mono text-xs tracking-[0.2em] uppercase text-rose'>
          // {t('upstream index')}
        </span>
        <h2 className='mt-4 text-[clamp(34px,4.5vw,52px)] font-bold leading-[1.05] tracking-[-0.035em]'>
          {t('Many nodes.')}{' '}
          <span className='bg-gradient-to-br from-violet via-blue to-cyan bg-clip-text italic [font-family:var(--font-display)] font-normal text-transparent'>
            {t('One graph.')}.
          </span>
        </h2>
        <p className='text-muted-foreground mx-auto mt-4 max-w-[560px] text-base'>
          {t(
            'Each upstream arrives on its own terms. YSRouter normalizes them into a single routable mesh.'
          )}
        </p>
      </div>

      <div className='relative grid grid-cols-3 gap-2.5 sm:grid-cols-4 md:grid-cols-6'>
        {PROVIDERS.map((p) => (
          <div
            key={p.name}
            className='bg-muted/30 relative flex aspect-square flex-col items-center justify-center gap-1.5 rounded-xl border transition duration-200 hover:-translate-y-0.5 hover:bg-muted/50'
            style={{ '--node-color': p.color } as React.CSSProperties}
          >
            <span className='absolute right-2 top-2 size-1.5 rounded-full bg-emerald-500 shadow-[0_0_8px] shadow-emerald-500' />
            <span
              className='grid size-9 place-items-center rounded-full font-medium not-italic [font-family:var(--font-display)] text-[18px]'
              style={{
                background:
                  'color-mix(in srgb, var(--node-color) 15%, transparent)',
                border:
                  '1px solid color-mix(in srgb, var(--node-color) 40%, transparent)',
                color: 'var(--node-color)',
              }}
            >
              {p.glyph}
            </span>
            <span className='font-mono text-[11px] tracking-[0.02em] text-muted-foreground'>
              {p.name}
            </span>
          </div>
        ))}
        <div
          className='flex aspect-square flex-col items-center justify-center gap-1.5 rounded-xl border border-dashed border-border'
          style={{ '--node-color': 'var(--color-rose)' } as React.CSSProperties}
        >
          <span
            className='grid size-9 place-items-center rounded-full font-mono text-sm font-semibold'
            style={{
              background:
                'color-mix(in srgb, var(--node-color) 15%, transparent)',
              border:
                '1px solid color-mix(in srgb, var(--node-color) 40%, transparent)',
              color: 'var(--node-color)',
            }}
          >
            +30
          </span>
          <span className='font-mono text-[11px] tracking-[0.02em] text-muted-foreground'>
            {t('more integrated')}
          </span>
        </div>
      </div>
    </section>
  )
}
