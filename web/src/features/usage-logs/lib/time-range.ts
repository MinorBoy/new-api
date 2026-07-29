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
import dayjs from '@/lib/dayjs'

export type UsageLogTimeRangePreset =
  | 'last24Hours'
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
export function getDefaultTimeRange(
  now: Date = new Date()
): { start: Date; end: Date } {
  return getUsageLogTimeRangePreset('last24Hours', now)
}
