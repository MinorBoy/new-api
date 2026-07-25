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
const PERCENT_INPUT_PATTERN = /^(?:\d+)(?:\.\d{1,2})?$/

export function marginPercentInputToBPS(value: string): number {
  const trimmed = value.trim()
  if (!PERCENT_INPUT_PATTERN.test(trimmed)) {
    throw new Error(
      'Enter a percentage from 0 to 100 with at most two decimals'
    )
  }
  const percent = Number(trimmed)
  if (!Number.isFinite(percent) || percent < 0 || percent > 100) {
    throw new Error(
      'Enter a percentage from 0 to 100 with at most two decimals'
    )
  }
  return Math.round(percent * 100)
}

export function formatMarginBPSPercent(value: number): string {
  const percent = value / 100
  return Number.isInteger(percent)
    ? String(percent)
    : percent.toFixed(2).replace(/0+$/, '').replace(/\.$/, '')
}
