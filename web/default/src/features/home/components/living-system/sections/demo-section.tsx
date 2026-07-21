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

import { HeroTerminalDemo } from '../../hero-terminal-demo'

/**
 * Demo section — the gallery-framed terminal showing a live routing
 * request. Reuses the existing HeroTerminalDemo component (interactive
 * curl/python/node tabs) and wraps it in the Living System's
 * "Art × UI" double-border gallery frame with a violet/teal radial
 * gradient backdrop and a centered figure caption.
 */
export function DemoSection() {
  const { t } = useTranslation()

  return (
    <section
      id='demo'
      className='mx-auto max-w-[1240px] px-7 py-25'
      style={{ fontFamily: 'var(--font-geist)' }}
    >
      <div className='mx-auto mb-16 max-w-[680px] text-center'>
        <span className='inline-flex items-center gap-2 rounded-full border border-cyan/30 bg-cyan/10 px-2.5 py-1 font-mono text-xs tracking-[0.2em] uppercase text-cyan'>
          // {t('live demo')}
        </span>
        <h2 className='mt-4 text-[clamp(34px,4.5vw,52px)] font-bold leading-[1.05] tracking-[-0.035em]'>
          {t('See it')}{' '}
          <span className='bg-gradient-to-br from-violet via-blue to-cyan bg-clip-text italic [font-family:var(--font-display)] font-normal text-transparent'>
            {t('route.')}.
          </span>
        </h2>
        <p className='text-muted-foreground mx-auto mt-4 max-w-[560px] text-base'>
          {t(
            'A request leaves your client, fans out to the cheapest healthy upstream, and returns — metered, logged, cached.'
          )}
        </p>
      </div>

      <div className='relative py-7.5'>
        <div
          className='relative overflow-hidden rounded-3xl border border-border bg-muted/30 p-6 md:p-12'
          style={{
            background:
              'radial-gradient(circle at 20% 20%, color-mix(in srgb, var(--color-violet) 12%, transparent), transparent 50%),' +
              'radial-gradient(circle at 80% 80%, color-mix(in srgb, var(--color-cyan) 10%, transparent), transparent 50%)',
          }}
        >
          <HeroTerminalDemo />
        </div>
        <p className='mt-5 text-center font-mono text-xs tracking-[0.1em] text-muted-foreground/70'>
          {t('FIG. 01 — A SINGLE REQUEST, FULLY INSTRUMENTED')}
        </p>
      </div>
    </section>
  )
}
