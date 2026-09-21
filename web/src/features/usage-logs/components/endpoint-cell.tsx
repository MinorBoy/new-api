import { useTranslation } from 'react-i18next'

import type { UsageLog } from '../data/schema'
import { parseLogOther } from '../lib/format'

export function EndpointCell({ log }: { log: UsageLog }) {
  const { t } = useTranslation()
  const other = parseLogOther(log.other)
  const inboundPath = other?.request_path
  const upstreamPath = other?.upstream_request_path

  if (!inboundPath && !upstreamPath) {
    return <span className='text-muted-foreground/60 text-xs'>-</span>
  }

  return (
    <div className='flex max-w-[240px] flex-col gap-0.5 font-mono text-xs'>
      {inboundPath ? (
        <span className='truncate' title={inboundPath}>
          {t('Inbound')} {inboundPath}
        </span>
      ) : null}
      {upstreamPath ? (
        <span className='text-muted-foreground truncate' title={upstreamPath}>
          {t('Upstream')}: {upstreamPath}
        </span>
      ) : null}
    </div>
  )
}
