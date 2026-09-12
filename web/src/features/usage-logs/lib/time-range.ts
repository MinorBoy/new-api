import dayjs from '@/lib/dayjs'

export type UsageLogTimeRangePreset =
  | 'last24Hours'
  | 'yesterday'
  | 'today'
  | '7d'
  | 'week'
  | '30d'
  | 'month'
  | 'lastMonth'

export function getUsageLogTimeRangePreset(
  preset: UsageLogTimeRangePreset,
  now: Date = new Date()
): { start: Date; end: Date } {
  const current = dayjs(now)

  switch (preset) {
    case 'last24Hours':
      return {
        start: current.subtract(24, 'hour').toDate(),
        end: current.toDate(),
      }
    case 'yesterday': {
      const yesterday = current.subtract(1, 'day')
      return {
        start: yesterday.startOf('day').toDate(),
        end: yesterday.endOf('day').toDate(),
      }
    }
    case 'today':
      return {
        start: current.startOf('day').toDate(),
        end: current.endOf('day').toDate(),
      }
    case '7d':
      return {
        start: current.subtract(6, 'day').startOf('day').toDate(),
        end: current.endOf('day').toDate(),
      }
    case 'week':
      return {
        start: current.startOf('week').toDate(),
        end: current.endOf('week').toDate(),
      }
    case '30d':
      return {
        start: current.subtract(29, 'day').startOf('day').toDate(),
        end: current.endOf('day').toDate(),
      }
    case 'month':
      return {
        start: current.startOf('month').toDate(),
        end: current.endOf('month').toDate(),
      }
    case 'lastMonth': {
      const lastMonth = current.subtract(1, 'month')
      return {
        start: lastMonth.startOf('month').toDate(),
        end: lastMonth.endOf('month').toDate(),
      }
    }
  }
}

/**
 * Default usage-log range: the rolling 24 hours ending at the current moment.
 */
export function getDefaultTimeRange(now: Date = new Date()): {
  start: Date
  end: Date
} {
  return getUsageLogTimeRangePreset('last24Hours', now)
}
