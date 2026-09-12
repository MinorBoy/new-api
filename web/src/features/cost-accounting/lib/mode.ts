import type { CostAccountingMode } from '../types'

export function isCostAccountingMode(
  value: string
): value is CostAccountingMode {
  return value === 'disabled' || value === 'tracking' || value === 'strict'
}
