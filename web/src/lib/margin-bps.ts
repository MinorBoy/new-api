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
