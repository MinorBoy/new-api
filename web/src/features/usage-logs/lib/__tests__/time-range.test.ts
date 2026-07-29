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
import assert from 'node:assert/strict'
import { describe, test } from 'node:test'

import {
  getDefaultTimeRange,
  getUsageLogTimeRangePreset,
} from '../time-range'

describe('usage log time ranges', () => {
  test('defaults to the rolling 24 hours ending at the current moment', () => {
    const now = new Date(2026, 6, 30, 14, 30, 45)

    const range = getDefaultTimeRange(now)

    assert.equal(range.start.getTime(), now.getTime() - 24 * 60 * 60 * 1000)
    assert.equal(range.end.getTime(), now.getTime())
  })

  test('uses the complete previous calendar month for the last-month preset', () => {
    const range = getUsageLogTimeRangePreset(
      'lastMonth',
      new Date(2026, 6, 30, 14, 30, 45)
    )

    assert.deepEqual(range.start, new Date(2026, 5, 1, 0, 0, 0, 0))
    assert.deepEqual(range.end, new Date(2026, 5, 30, 23, 59, 59, 999))
  })
})
