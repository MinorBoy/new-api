import type { DataTableColumnClassName } from '@/components/data-table'
import { cn } from '@/lib/utils'

import type { LogCategory } from '../types'

interface UsageLogsTableDensity {
  rowClassName: string
  getColumnClassName: DataTableColumnClassName
}

const taskAuditColumnIds = new Set([
  'user_request_data',
  'upstream_response_data',
  'user_response_data',
])

const compactColumnClassName: DataTableColumnClassName = (columnId, kind) => {
  if (kind === 'header') {
    return cn(
      'px-1.5',
      taskAuditColumnIds.has(columnId) &&
        'h-auto min-h-10 whitespace-normal py-2 leading-tight'
    )
  }

  return 'px-1.5 py-2.5'
}

const commonColumnClassName: DataTableColumnClassName = (_columnId, kind) =>
  kind === 'cell' ? 'py-2' : undefined

const compactDensity: UsageLogsTableDensity = {
  rowClassName: 'h-13!',
  getColumnClassName: compactColumnClassName,
}

const commonDensity: UsageLogsTableDensity = {
  rowClassName: '',
  getColumnClassName: commonColumnClassName,
}

export function getUsageLogsTableDensity(
  logCategory: LogCategory
): UsageLogsTableDensity {
  return logCategory === 'common' ? commonDensity : compactDensity
}
