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
import test from 'node:test'

import type { TFunction } from 'i18next'

import type { UsageLog } from '../../data/schema'
import type { LogOtherData } from '../../types'
import { getBillingBreakdownRows } from '../dialogs/billing-breakdown'

const log: UsageLog = {
  id: 1,
  user_id: 1,
  created_at: 1,
  type: 2,
  content: '',
  username: '',
  token_name: '',
  model_name: '',
  quota: 1_250_000,
  prompt_tokens: 0,
  completion_tokens: 0,
  use_time: 0,
  is_stream: false,
  channel: 5,
  channel_name: 'dimensio',
  token_id: 1,
  group: 'default',
  ip: '',
  other: '',
  request_id: 'duration-log',
  upstream_request_id: '',
}
const identityTranslation = ((key: string) => key) as unknown as TFunction

test('renders duration billing without a token pricing fallback', () => {
  const rows = getBillingBreakdownRows(
    log,
    {
      billing_mode: 'per_duration',
      duration_price: 0.25,
      requested_duration_seconds: 5,
      billable_duration_seconds: 5,
    } as LogOtherData,
    false,
    identityTranslation
  )

  assert.deepEqual(
    rows.filter((row) =>
      ['Billing Mode', 'Duration Price', 'Billable Duration'].includes(
        row.label
      )
    ),
    [
      { label: 'Billing Mode', value: 'Per-duration' },
      { label: 'Duration Price', value: '$0.25/s' },
      { label: 'Billable Duration', value: '5 s' },
    ]
  )
  assert.equal(
    rows.some((row) => row.value === 'Per-token'),
    false
  )
})
