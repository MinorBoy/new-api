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
import { ArrowRight, Check } from 'lucide-react'
import { useTranslation } from 'react-i18next'

const BILL_ROWS = [
  { name: 'Claude Sonnet 4', usage: '1.2M tok', cost: '$3.42' },
  { name: 'GPT-4o', usage: '4.8M tok', cost: '$14.40' },
  { name: 'DeepSeek V3', usage: '9.1M tok', cost: '$1.82' },
  { name: 'Gemini 2.0 Flash', usage: '2.3M tok', cost: '$0.46' },
] as const

const FEATURES = [
  {
    titleKey: 'Pre-pay, deduct per call.',
    descKey:
      'Top up your balance once; every request is metered and deducted to the cent.',
  },
  {
    titleKey: 'Transparent pass-through.',
    descKey:
      "Each model's price matches its upstream exactly. No markup, ever.",
  },
  {
    titleKey: 'Self-host free, forever.',
    descKey:
      'Run the gateway on your own infra — pricing is between you and providers.',
  },
] as const

/**
 * Pay-as-you-go pricing (NO subscription tiers). Two-column: main card
 * with the rate + CTA, and a side ledger showing a sample monthly bill
 * with a "balance after" total. Three small feature cards underneath
 * reinforce the no-markup / no-commitment positioning.
 */
export function PricingSection() {
  const { t } = useTranslation()

  return (
    <section
      id='pricing'
      className='mx-auto max-w-[1240px] px-7 py-25'
      style={{ fontFamily: 'var(--font-geist)' }}
    >
      <div className='mx-auto mb-16 max-w-[680px] text-center'>
        <span className='border-emerald/30 bg-emerald/10 text-emerald inline-flex items-center gap-2 rounded-full border px-2.5 py-1 font-mono text-xs tracking-[0.2em] uppercase'>
          // {t('pricing')}
        </span>
        <h2 className='mt-4 text-[clamp(34px,4.5vw,52px)] leading-[1.05] font-bold tracking-[-0.035em]'>
          {t('Pay for what you')}{' '}
          <span className='from-violet via-blue to-cyan bg-gradient-to-br bg-clip-text [font-family:var(--font-display)] font-normal text-transparent italic'>
            {t('route.')}.
          </span>
        </h2>
        <p className='text-muted-foreground mx-auto mt-4 max-w-[560px] text-base'>
          {t(
            'No tiers. No seats. No commitment. Metered to the token at provider cost.'
          )}
        </p>
      </div>

      <div className='mx-auto grid max-w-[1000px] grid-cols-1 gap-4.5 md:grid-cols-[1.3fr_1fr]'>
        {/* Main card */}
        <div
          className='border-border relative overflow-hidden rounded-3xl border p-8 md:p-10'
          style={{
            background:
              'radial-gradient(circle at 80% 0%, color-mix(in srgb, var(--color-violet) 18%, transparent), transparent 60%)',
          }}
        >
          <div className='relative'>
            <div className='text-violet font-mono text-xs font-semibold tracking-[0.2em]'>
              {t('PAY · AS · YOU · GO')}
            </div>
            <div className='mt-4 mb-3 flex items-baseline gap-1.5'>
              <span className='text-muted-foreground text-2xl font-semibold'>
                $
              </span>
              <span className='from-violet via-blue to-cyan bg-gradient-to-br bg-clip-text text-[64px] leading-none font-extrabold tracking-[-0.05em] text-transparent'>
                0.0021
              </span>
              <span className='text-muted-foreground font-mono text-sm'>
                / 1K tokens
              </span>
            </div>
            <p className='text-muted-foreground mb-6 max-w-[380px] text-[14.5px] leading-relaxed'>
              {t(
                'Priced per token, passed through from each upstream at their rate.'
              )}{' '}
              <strong className='text-foreground font-semibold'>
                {t('YSRouter adds no markup')}
              </strong>
              {t(' — you see exactly what the provider charges.')}
            </p>
            <Link
              to='/sign-up'
              className='from-violet via-blue to-cyan inline-flex items-center gap-2 rounded-[10px] bg-gradient-to-br px-4 py-2 font-semibold text-white shadow-[0_10px_30px_-8px_var(--color-violet)] transition hover:-translate-y-px hover:brightness-110'
            >
              {t('Top up & start routing')}
              <ArrowRight className='size-3.5' />
            </Link>
          </div>
        </div>

        {/* Sample bill */}
        <div className='bg-muted/30 border-border rounded-3xl border p-7 font-mono'>
          <h4 className='text-muted-foreground/70 mb-3.5 flex items-center justify-between text-xs tracking-[0.14em] uppercase'>
            <span>{t('sample usage · this month')}</span>
            <span className='size-1.5 rounded-full bg-emerald-500 shadow-[0_0_8px] shadow-emerald-500' />
          </h4>
          {BILL_ROWS.map((row) => (
            <div
              key={row.name}
              className='border-border text-muted-foreground grid grid-cols-[1.5fr_0.8fr_0.6fr] gap-1.5 border-b border-dotted py-2 text-[12.5px]'
            >
              <span className='text-foreground/80'>{row.name}</span>
              <span>{row.usage}</span>
              <span className='text-right'>{row.cost}</span>
            </div>
          ))}
          <div className='border-foreground mt-3.5 flex items-baseline justify-between border-t-2 pt-3.5'>
            <span className='text-muted-foreground text-xs'>
              {t('balance after')}
            </span>
            <span className='from-violet via-blue to-cyan bg-gradient-to-br bg-clip-text text-xl font-bold tracking-[-0.03em] text-transparent'>
              $80.10
            </span>
          </div>
          <div className='text-muted-foreground/60 mt-auto pt-3.5 text-center font-mono text-[10px] tracking-[0.08em]'>
            {t('no markup · no seat fees · no commitment')}
          </div>
        </div>
      </div>

      {/* Three feature cards */}
      <div className='mx-auto mt-5.5 grid max-w-[1000px] grid-cols-1 gap-3.5 md:grid-cols-3'>
        {FEATURES.map((f) => (
          <div
            key={f.titleKey}
            className='bg-muted/30 border-border flex items-start gap-2.75 rounded-2xl border p-4.5'
          >
            <Check className='text-emerald mt-0.5 size-4 shrink-0' />
            <div className='text-muted-foreground text-[13px] leading-relaxed'>
              <strong className='text-foreground font-semibold'>
                {t(f.titleKey)}
              </strong>{' '}
              {t(f.descKey)}
            </div>
          </div>
        ))}
      </div>
    </section>
  )
}
