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
import { Link } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'

import { getLobeIcon } from '@/lib/lobe-icon'

interface Provider {
  // lobehub dotted key, e.g. 'OpenAI' (mono) or 'Gemini.Color' (full color).
  // The key matches what the /pricing model square renders for the same
  // vendor, so the homepage wall and the catalog stay visually consistent.
  icon: string
  name: string
}

const PROVIDERS: Provider[] = [
  { icon: 'OpenAI', name: 'OpenAI' },
  { icon: 'Anthropic', name: 'Anthropic' },
  { icon: 'Gemini.Color', name: 'Gemini' },
  { icon: 'Mistral.Color', name: 'Mistral' },
  { icon: 'DeepSeek.Color', name: 'DeepSeek' },
  { icon: 'XAI', name: 'xAI' },
  { icon: 'Cohere.Color', name: 'Cohere' },
  { icon: 'Moonshot', name: 'Moonshot' },
  { icon: 'Qwen.Color', name: 'Qwen' },
  { icon: 'Zhipu.Color', name: 'Zhipu' },
  { icon: 'Doubao.Color', name: 'Doubao' },
  { icon: 'Yi.Color', name: 'Yi' },
  { icon: 'Baichuan.Color', name: 'Baichuan' },
  { icon: 'Minimax.Color', name: 'Minimax' },
  { icon: 'Spark.Color', name: 'Spark' },
  { icon: 'Azure.Color', name: 'Azure' },
  { icon: 'Bedrock.Color', name: 'Bedrock' },
]

/**
 * Model wall — the routable mesh of upstreams. Each cell shows the
 * provider's real brand logo (via getLobeIcon, same pipeline as the
 * /pricing model square) with a small green status dot to read as
 * "online". The last cell is a dashed link into the full model square.
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
        <span className='border-rose/30 bg-rose/10 text-rose inline-flex items-center gap-2 rounded-full border px-2.5 py-1 font-mono text-xs tracking-[0.2em] uppercase'>
          // {t('upstream index')}
        </span>
        <h2 className='mt-4 text-[clamp(34px,4.5vw,52px)] leading-[1.05] font-bold tracking-[-0.035em]'>
          {t('Many nodes.')}{' '}
          <span className='from-violet via-blue to-cyan bg-gradient-to-br bg-clip-text [font-family:var(--font-display)] font-normal text-transparent italic'>
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
            className='bg-muted/30 hover:bg-muted/50 relative flex aspect-square flex-col items-center justify-center gap-2 rounded-xl border transition duration-200'
          >
            <span className='absolute top-2 right-2 size-1.5 rounded-full bg-emerald-500 shadow-[0_0_8px] shadow-emerald-500' />
            <span className='grid size-9 place-items-center'>
              {getLobeIcon(p.icon, 28)}
            </span>
            <span className='text-muted-foreground font-mono text-[11px] tracking-[0.02em]'>
              {p.name}
            </span>
          </div>
        ))}
        <Link
          to='/pricing'
          className='border-border hover:bg-muted/50 hover:border-rose/40 group relative flex aspect-square cursor-pointer flex-col items-center justify-center gap-2 rounded-xl border border-dashed transition duration-200'
        >
          <span className='from-violet via-blue to-cyan grid size-9 place-items-center rounded-full bg-gradient-to-br font-mono text-sm font-semibold text-white transition duration-200 group-hover:scale-105'>
            +
          </span>
          <span className='text-muted-foreground font-mono text-[11px] tracking-[0.02em]'>
            {t('more integrated')}
          </span>
        </Link>
      </div>
    </section>
  )
}
