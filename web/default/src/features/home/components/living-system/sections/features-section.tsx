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
import {
  Code2,
  DollarSign,
  Globe,
  Server,
  ShieldCheck,
  Zap,
  type LucideIcon,
} from 'lucide-react'

interface Feature {
  num: string
  icon: LucideIcon
  color: string
  titleKey: string
  descKey: string
}

const FEATURES: Feature[] = [
  {
    num: '01',
    icon: Zap,
    color: 'var(--color-violet)',
    titleKey: 'Lightning routing',
    descKey:
      'Millisecond fan-out across upstreams. Stream tokens back with zero added latency.',
  },
  {
    num: '02',
    icon: ShieldCheck,
    color: 'var(--color-blue)',
    titleKey: 'Secure by default',
    descKey:
      'Per-key scopes, quotas, audit logs, SSO. Enterprise auth at every junction.',
  },
  {
    num: '03',
    icon: Globe,
    color: 'var(--color-cyan)',
    titleKey: 'Global coverage',
    descKey: 'Multi-region deployment for stable access from any continent.',
  },
  {
    num: '04',
    icon: DollarSign,
    color: 'var(--color-emerald)',
    titleKey: 'Transparent billing',
    descKey:
      'Pay per token at provider rate. No markup. Auditable per request.',
  },
  {
    num: '05',
    icon: Code2,
    color: 'var(--color-amber)',
    titleKey: 'Developer-first',
    descKey:
      'OpenAI, Claude, Gemini, Responses APIs — all on one base URL.',
  },
  {
    num: '06',
    icon: Server,
    color: 'var(--color-rose)',
    titleKey: 'Self-host, free',
    descKey:
      'AGPL-3.0. Run on your own infra. Pricing is between you and providers.',
  },
]

/**
 * Six-card bento — the "explosion of color" of the Living System. Each
 * card carries a local `--card-color` token that drives its icon tile,
 * top accent bar, and hover border, so the six raw palette colors read
 * as a coordinated set rather than six ad-hoc tints.
 */
export function FeaturesSection() {
  const { t } = useTranslation()

  return (
    <section
      id='features'
      className='mx-auto max-w-[1240px] px-7 py-25'
      style={{ fontFamily: 'var(--font-geist)' }}
    >
      <div className='mx-auto mb-16 max-w-[680px] text-center'>
        <span className='inline-flex items-center gap-2 rounded-full border border-amber/30 bg-amber/10 px-2.5 py-1 font-mono text-xs tracking-[0.2em] uppercase text-amber'>
          // {t('capabilities')}
        </span>
        <h2 className='mt-4 text-[clamp(34px,4.5vw,52px)] font-bold leading-[1.05] tracking-[-0.035em]'>
          {t('Ten primitives,')}{' '}
          <span className='bg-gradient-to-br from-violet via-blue to-cyan bg-clip-text italic [font-family:var(--font-display)] font-normal text-transparent'>
            {t('one deploy.')}.
          </span>
        </h2>
        <p className='text-muted-foreground mx-auto mt-4 max-w-[560px] text-base'>
          {t(
            'Everything an AI gateway should ship with. Switched on or off per channel.'
          )}
        </p>
      </div>

      <div className='grid grid-cols-1 gap-4 sm:grid-cols-2 lg:grid-cols-3'>
        {FEATURES.map((f) => {
          const Icon = f.icon
          return (
            <div
              key={f.num}
              className='bg-muted/30 relative cursor-default overflow-hidden rounded-2xl border border-border p-7 transition duration-300 hover:-translate-y-[3px] hover:bg-muted/50'
              style={{ '--card-color': f.color } as React.CSSProperties}
            >
              <span className='absolute right-6 top-6 font-mono text-xs font-medium text-muted-foreground/70'>
                {f.num}
              </span>
              <div
                className='mb-4.5 grid size-10 place-items-center rounded-[10px] transition duration-300'
                style={{
                  background:
                    'color-mix(in srgb, var(--card-color) 15%, transparent)',
                  border:
                    '1px solid color-mix(in srgb, var(--card-color) 30%, transparent)',
                  color: 'var(--card-color)',
                }}
              >
                <Icon className='size-[18px]' />
              </div>
              <h3 className='mb-2 text-[17px] font-semibold tracking-[-0.02em]'>
                {t(f.titleKey)}
              </h3>
              <p className='text-muted-foreground text-[13.5px] leading-relaxed'>
                {t(f.descKey)}
              </p>
              <span
                className='absolute left-0 right-0 top-0 h-[3px] opacity-60 transition duration-300 group-hover:h-[5px] group-hover:opacity-100'
                style={{ background: 'var(--card-color)' }}
              />
            </div>
          )
        })}
      </div>
    </section>
  )
}
