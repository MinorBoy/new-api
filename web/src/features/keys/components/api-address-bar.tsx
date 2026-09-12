import { Zap } from 'lucide-react'
import { useCallback, useState } from 'react'
import { useTranslation } from 'react-i18next'

import { CopyButton } from '@/components/copy-button'
import { StatusBadge } from '@/components/status-badge'
import { Button } from '@/components/ui/button'
import { useApiInfo } from '@/features/dashboard/hooks/use-status-data'
import {
  getDefaultPingStatus,
  getLatencyColorClass,
  testUrlLatency,
} from '@/features/dashboard/lib/api-info'
import type { ApiInfoItem, PingStatusMap } from '@/features/dashboard/types'
import { getBgColorClass } from '@/lib/colors'
import { cn } from '@/lib/utils'

export function ApiAddressBar() {
  const { t } = useTranslation()
  const { items } = useApiInfo()
  const [pingStatus, setPingStatus] = useState<PingStatusMap>({})

  const handleTest = useCallback(async (url: string) => {
    setPingStatus((prev) => ({
      ...prev,
      [url]: { latency: null, testing: true, error: false },
    }))
    const result = await testUrlLatency(url)
    setPingStatus((prev) => ({ ...prev, [url]: result }))
  }, [])

  if (items.length === 0) return null

  return (
    <div className='bg-muted/30 divide-border/60 divide-y overflow-hidden rounded-lg border'>
      {items.map((item: ApiInfoItem) => {
        const status = pingStatus[item.url] || getDefaultPingStatus()
        return (
          <div
            key={item.url}
            className='group hover:bg-muted/40 flex items-center justify-between gap-2 px-3 py-2 transition-colors sm:gap-3 sm:px-4'
          >
            <div className='flex min-w-0 flex-1 items-center gap-2 sm:gap-3'>
              <span
                className={cn(
                  'inline-block size-2 shrink-0 rounded-full',
                  getBgColorClass(item.color)
                )}
              />
              <div className='flex min-w-0 flex-1 flex-col gap-0.5'>
                <div className='flex items-baseline gap-2'>
                  <span className='font-mono text-sm font-semibold'>
                    {item.route}
                  </span>
                  <span className='text-muted-foreground/60 hidden truncate text-xs md:inline'>
                    {item.description}
                  </span>
                </div>
                <span className='text-muted-foreground/40 truncate font-mono text-xs'>
                  {item.url}
                </span>
              </div>
            </div>

            <div className='flex shrink-0 items-center gap-2'>
              <div className='flex items-center'>
                {status.testing && (
                  <StatusBadge
                    label={t('Testing...')}
                    variant='warning'
                    className='animate-pulse'
                    copyable={false}
                  />
                )}
                {status.latency !== null && !status.testing && (
                  <StatusBadge
                    variant='success'
                    label={`${status.latency}${t('ms')}`}
                    className={cn(
                      'font-mono font-medium',
                      getLatencyColorClass(status.latency)
                    )}
                    copyable={false}
                  />
                )}
                {status.error && (
                  <StatusBadge
                    label={t('N/A')}
                    variant='neutral'
                    copyable={false}
                  />
                )}
              </div>

              <div className='flex items-center gap-0.5'>
                <Button
                  variant='ghost'
                  size='sm'
                  onClick={() => handleTest(item.url)}
                  disabled={status.testing}
                  className='size-7 p-0'
                  title={t('Test Latency')}
                >
                  <Zap
                    className={cn(
                      'size-3.5',
                      status.testing && 'animate-pulse'
                    )}
                  />
                </Button>

                <CopyButton
                  value={item.url}
                  variant='ghost'
                  size='sm'
                  className='size-7 p-0'
                  iconClassName='size-3.5'
                  tooltip={t('Copy URL')}
                  aria-label={t('Copy URL')}
                />
              </div>
            </div>
          </div>
        )
      })}
    </div>
  )
}
