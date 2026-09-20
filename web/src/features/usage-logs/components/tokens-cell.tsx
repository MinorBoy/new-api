import { Archive, ArrowDown, ArrowUp } from 'lucide-react'
import { useTranslation } from 'react-i18next'

import {
  Tooltip,
  TooltipContent,
  TooltipProvider,
  TooltipTrigger,
} from '@/components/ui/tooltip'

import type { UsageLog } from '../data/schema'
import { parseLogOther } from '../lib/format'
import { isDisplayableLogType } from '../lib/utils'

/**
 * Token counts render in full below 100K (47,562) and switch to a compact
 * K/M suffix from 100K up (241.4K, 1.25M) so wide usage stays scannable.
 */
function formatTokenCount(tokens: number): string {
  if (tokens >= 1_000_000) {
    return `${(tokens / 1_000_000).toFixed(2).replace(/\.?0+$/, '')}M`
  }
  if (tokens >= 100_000) {
    return `${(tokens / 1_000).toFixed(1).replace(/\.0$/, '')}K`
  }
  return tokens.toLocaleString()
}

export function TokensCell({ log }: { log: UsageLog }) {
  const { t } = useTranslation()

  if (!isDisplayableLogType(log.type)) return null

  const promptTokens = log.prompt_tokens || 0
  const completionTokens = log.completion_tokens || 0
  if (promptTokens === 0 && completionTokens === 0) {
    return <span className='text-muted-foreground text-xs'>-</span>
  }

  const other = parseLogOther(log.other)
  const cacheReadTokens = other?.cache_tokens || 0
  const cacheWrite5m = other?.cache_creation_tokens_5m || 0
  const cacheWrite1h = other?.cache_creation_tokens_1h || 0
  const hasSplitCache = cacheWrite5m > 0 || cacheWrite1h > 0
  const cacheWriteTokens = hasSplitCache
    ? cacheWrite5m + cacheWrite1h
    : other?.cache_creation_tokens || 0
  const cacheTotalTokens = cacheReadTokens + cacheWriteTokens

  return (
    <div className='flex flex-col gap-0.5'>
      <div className='flex items-center gap-2 font-mono text-xs font-medium tabular-nums'>
        <span
          className='inline-flex items-center gap-0.5'
          aria-label={t('Input Tokens')}
        >
          <ArrowDown className='size-3 text-emerald-500' aria-hidden='true' />
          {formatTokenCount(promptTokens)}
        </span>
        <span
          className='inline-flex items-center gap-0.5'
          aria-label={t('Output Tokens')}
        >
          <ArrowUp className='size-3 text-violet-500' aria-hidden='true' />
          {formatTokenCount(completionTokens)}
        </span>
      </div>
      {cacheTotalTokens > 0 && (
        <TooltipProvider delay={300}>
          <Tooltip>
            <TooltipTrigger
              render={
                <div className='text-muted-foreground flex w-fit cursor-default items-center gap-1 text-[11px]' />
              }
            >
              <Archive
                className='size-3 text-cyan-500'
                aria-label={t('Cache')}
              />
              <span className='font-mono tabular-nums'>
                {formatTokenCount(cacheTotalTokens)}
              </span>
            </TooltipTrigger>
            <TooltipContent side='top' className='text-xs'>
              <div className='space-y-0.5'>
                {cacheReadTokens > 0 && (
                  <p className='flex items-center justify-between gap-3'>
                    <span className='text-muted-foreground'>
                      {t('Cache Read')}
                    </span>
                    <span className='font-mono tabular-nums'>
                      {cacheReadTokens.toLocaleString()}
                    </span>
                  </p>
                )}
                {hasSplitCache ? (
                  <>
                    {cacheWrite5m > 0 && (
                      <p className='flex items-center justify-between gap-3'>
                        <span className='text-muted-foreground'>
                          {t('Cache Write (5m)')}
                        </span>
                        <span className='font-mono tabular-nums'>
                          {cacheWrite5m.toLocaleString()}
                        </span>
                      </p>
                    )}
                    {cacheWrite1h > 0 && (
                      <p className='flex items-center justify-between gap-3'>
                        <span className='text-muted-foreground'>
                          {t('Cache Write (1h)')}
                        </span>
                        <span className='font-mono tabular-nums'>
                          {cacheWrite1h.toLocaleString()}
                        </span>
                      </p>
                    )}
                  </>
                ) : (
                  cacheWriteTokens > 0 && (
                    <p className='flex items-center justify-between gap-3'>
                      <span className='text-muted-foreground'>
                        {t('Cache Write')}
                      </span>
                      <span className='font-mono tabular-nums'>
                        {cacheWriteTokens.toLocaleString()}
                      </span>
                    </p>
                  )
                )}
              </div>
            </TooltipContent>
          </Tooltip>
        </TooltipProvider>
      )}
    </div>
  )
}
