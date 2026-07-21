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
import { motion, useReducedMotion } from 'motion/react'
import { useTranslation } from 'react-i18next'
import { ArrowRight, BookOpen } from 'lucide-react'

import { useStatus } from '@/hooks/use-status'

import { RouteCanvas } from '../route-canvas'

interface HeroSectionProps {
  isAuthenticated?: boolean
}

/**
 * Hero of the Living System composition.
 *
 * Headline "One mesh. Every model," with each word animating in on a
 * 0.12s stagger using motion variants. The accent word ("mesh.") uses
 * the project's gradient text class and the Instrument Serif display
 * face for contrast against the Geist sans of the rest of the headline.
 *
 * Reduced motion: words render statically at their final position.
 *
 * Below the CTAs sits <RouteCanvas />, the proprietary LIVE ROUTING
 * visual effect for the composition.
 */
export function HeroSection({ isAuthenticated }: HeroSectionProps) {
  const { t } = useTranslation()
  const { status } = useStatus()
  const docsLink =
    (status?.docs_link as string | undefined) || 'https://docs.newapi.pro'
  const shouldReduce = useReducedMotion()

  // Stagger the headline words on entry. Reduced motion renders them
  // statically — we keep the same JSX but skip the motion wrapper.
  // Two-segment headline so each piece localizes as a unit; the first
  // segment carries the accent style (gradient italic display face).
  const words = [
    { text: t('One mesh.'), accent: true },
    { text: t('Every model,'), accent: false },
  ]

  const containerVariants = {
    hidden: {},
    visible: {
      transition: { staggerChildren: 0.12, delayChildren: 0.2 },
    },
  }
  const wordVariants = {
    hidden: { opacity: 0, y: 20 },
    visible: {
      opacity: 1,
      y: 0,
      transition: { duration: 0.6, ease: [0.2, 0.8, 0.2, 1] as const },
    },
  }

  return (
    <header
      id='hero'
      className='relative px-7 pt-22 pb-15 text-center'
      style={{ fontFamily: 'var(--font-geist)' }}
    >
      <span className='border-border bg-muted/30 text-muted-foreground inline-flex items-center gap-2 rounded-full border px-3.5 py-1 pl-2.5 font-mono text-xs backdrop-blur'>
        <span className='inline-block size-1.5 rounded-full bg-emerald-500 shadow-[0_0_10px] shadow-emerald-500 animate-pulse' />
        {t('v3.0 · streaming · prompt caching · 50+ upstreams')}
      </span>

      <h1 className='mx-auto mt-7 max-w-[920px] text-[clamp(44px,7vw,84px)] font-extrabold leading-[0.98] tracking-[-0.045em]'>
        {shouldReduce ? (
          <>
            <span className='bg-gradient-to-br from-violet via-blue to-cyan bg-clip-text italic [font-family:var(--font-display)] font-medium text-transparent'>
              {t('One mesh.')}
            </span>
            <span> {t('Every model,')}</span>
          </>
        ) : (
          <motion.span
            variants={containerVariants}
            initial='hidden'
            animate='visible'
            className='inline'
          >
            {words.map((w) => (
              <motion.span
                key={w.text}
                variants={wordVariants}
                className={
                  w.accent
                    ? 'bg-gradient-to-br from-violet via-blue to-cyan bg-clip-text italic [font-family:var(--font-display)] font-medium text-transparent'
                    : ''
                }
              >
                {w.text}{' '}
              </motion.span>
            ))}
          </motion.span>
        )}
      </h1>

      <p className='text-muted-foreground mx-auto mt-7 max-w-[560px] text-lg leading-relaxed'>
        {t(
          'Fifty upstream AI services. One OpenAI-compatible URL. Zero client changes.'
        )}
      </p>

      <div className='mt-9 flex flex-wrap justify-center gap-3'>
        {isAuthenticated ? (
          <Link
            to='/dashboard'
            className='inline-flex items-center gap-2 rounded-[10px] bg-gradient-to-br from-violet via-blue to-cyan px-4 py-2 font-semibold text-white shadow-[0_8px_24px_-6px_var(--color-violet)] transition hover:-translate-y-px hover:brightness-110'
          >
            {t('Go to Dashboard')}
            <ArrowRight className='size-3.5' />
          </Link>
        ) : (
          <Link
            to='/sign-up'
            className='inline-flex items-center gap-2 rounded-[10px] bg-gradient-to-br from-violet via-blue to-cyan px-4 py-2 font-semibold text-white shadow-[0_8px_24px_-6px_var(--color-violet)] transition hover:-translate-y-px hover:brightness-110'
          >
            {t('Start routing')}
            <ArrowRight className='size-3.5' />
          </Link>
        )}
        <a
          href={`#${'demo'}`}
          className='border-border bg-muted/30 inline-flex items-center gap-2 rounded-[10px] border px-4 py-2 transition hover:bg-muted/50'
        >
          {t('Watch it route')}
        </a>
        <a
          href={docsLink}
          target='_blank'
          rel='noreferrer'
          className='border-border bg-muted/30 inline-flex items-center gap-2 rounded-[10px] border px-4 py-2 transition hover:bg-muted/50'
        >
          <BookOpen className='size-3.5' />
          {t('Read the docs')}
        </a>
      </div>

      <RouteCanvas className='mx-auto' />
    </header>
  )
}
