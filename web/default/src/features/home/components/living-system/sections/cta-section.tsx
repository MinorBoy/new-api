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
import { ArrowRight } from 'lucide-react'

interface CtaSectionProps {
  isAuthenticated?: boolean
}

/**
 * Final CTA — a radial-glow card closing the page. Mirrors the existing
 * default CTA behavior: hidden for authenticated visitors (they're
 * already in).
 */
export function CtaSection({ isAuthenticated }: CtaSectionProps) {
  const { t } = useTranslation()

  if (isAuthenticated) return null

  return (
    <section
      className='mx-auto max-w-[1240px] px-7 py-30 text-center'
      style={{ fontFamily: 'var(--font-geist)' }}
    >
      <div
        className='relative overflow-hidden rounded-3xl border border-border px-10 py-20'
        style={{
          background:
            'radial-gradient(circle at 50% 0%, color-mix(in srgb, var(--color-violet) 20%, transparent), transparent 60%)',
        }}
      >
        <h2 className='relative text-[clamp(34px,4.5vw,56px)] font-bold leading-[1.05] tracking-[-0.035em]'>
          {t('Start routing in')}{' '}
          <span className='bg-gradient-to-br from-violet via-blue to-cyan bg-clip-text italic [font-family:var(--font-display)] font-normal text-transparent'>
            {t('minutes.')}.
          </span>
        </h2>
        <p className='text-muted-foreground relative mx-auto mt-4.5 max-w-[520px] text-lg'>
          {t(
            'Deploy your own gateway. Swap one URL. The whole catalog, one mesh.'
          )}
        </p>
        <div className='relative mt-8 flex flex-wrap justify-center gap-3'>
          <Link
            to='/sign-up'
            className='inline-flex items-center gap-2 rounded-[10px] bg-gradient-to-br from-violet via-blue to-cyan px-4 py-2 font-semibold text-white shadow-[0_8px_24px_-6px_var(--color-violet)] transition hover:-translate-y-px hover:brightness-110'
          >
            {t('Get started')}
            <ArrowRight className='size-3.5' />
          </Link>
          <Link
            to='/pricing'
            className='border-border bg-muted/30 inline-flex items-center gap-2 rounded-[10px] border px-4 py-2 transition hover:bg-muted/50'
          >
            {t('See pricing')}
          </Link>
        </div>
      </div>
    </section>
  )
}
