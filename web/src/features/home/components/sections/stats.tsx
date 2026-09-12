import { useTranslation } from 'react-i18next'

import { Counter } from '@/components/counter'

interface StatsProps {
  className?: string
}

interface StatItem {
  end: number
  suffix: string
  label: string
  decimals?: number
}

export function Stats(_props: StatsProps) {
  const { t } = useTranslation()

  const stats: StatItem[] = [
    { end: 50, suffix: '+', label: t('upstream services integrated') },
    { end: 100, suffix: '+', label: t('model billing support') },
    { end: 50, suffix: '+', label: t('compatible API routes') },
    { end: 10, suffix: '+', label: t('scheduling controls') },
  ]

  return (
    <div className='border-border/40 bg-muted/10 relative z-10 border-y'>
      <div className='mx-auto max-w-6xl px-6 py-10 md:py-12'>
        <div className='grid grid-cols-2 gap-8 md:grid-cols-4 md:gap-12'>
          {stats.map((s) => (
            <div
              key={s.label}
              className='flex flex-col items-center text-center'
            >
              <span className='text-2xl font-bold tracking-tight md:text-3xl'>
                <Counter end={s.end} suffix={s.suffix} decimals={s.decimals} />
              </span>
              <span className='text-muted-foreground mt-1.5 text-xs'>
                {s.label}
              </span>
            </div>
          ))}
        </div>
      </div>
    </div>
  )
}
