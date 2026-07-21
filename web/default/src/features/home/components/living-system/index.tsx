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

import { Footer } from '@/components/layout/components/footer'
import { PublicLayout } from '@/components/layout'
import { cn } from '@/lib/utils'

import { ScrollRail } from './scroll-rail'
import { CtaSection } from './sections/cta-section'
import { DemoSection } from './sections/demo-section'
import { FeaturesSection } from './sections/features-section'
import { HeroSection } from './sections/hero-section'
import { ModelsSection } from './sections/models-section'
import { PricingSection } from './sections/pricing-section'
import { StatsSection } from './sections/stats-section'
import { TldrSection } from './sections/tldr-section'

// Segment ids must match the `id` attributes set on each section below.
// The TL;DR block sits inside the hero flow and is not a rail target.
const RAIL_SEGMENTS = [
  { id: 'hero', labelKey: 'Hero' },
  { id: 'demo', labelKey: 'Demo' },
  { id: 'features', labelKey: 'Features' },
  { id: 'models', labelKey: 'Models' },
  { id: 'pricing', labelKey: 'Pricing' },
] as const

interface LivingSystemHomeProps {
  isAuthenticated?: boolean
}

/**
 * Living System — the 2026-trend landing composition (A.02 from
 * docs/frontend/landing-ysrouter-2026.html). Selected by setting the
 * `home.style` admin option to 'living-system'; see
 * features/home/index.tsx for the switch.
 *
 * Wraps the eight sections in PublicLayout and pins a ScrollRail to
 * the left edge on large screens. The rail fades in once the visitor
 * scrolls past 200px (so the hero gets the full first-paint attention).
 */
export function LivingSystemHome({ isAuthenticated }: LivingSystemHomeProps) {
  const [railVisible, setRailVisible] = useState(false)

  useEffect(() => {
    const onScroll = () => setRailVisible(window.scrollY > 200)
    onScroll()
    window.addEventListener('scroll', onScroll, { passive: true })
    return () => window.removeEventListener('scroll', onScroll)
  }, [])

  return (
    <PublicLayout showMainContainer={false}>
      <div className={cn(railVisible && 'scroll-rail-visible')}>
        <ScrollRail segments={RAIL_SEGMENTS} />
        <HeroSection isAuthenticated={isAuthenticated} />
        <TldrSection />
        <DemoSection />
        <FeaturesSection />
        <StatsSection />
        <ModelsSection />
        <PricingSection />
        <CtaSection isAuthenticated={isAuthenticated} />
        <Footer />
      </div>
    </PublicLayout>
  )
}
