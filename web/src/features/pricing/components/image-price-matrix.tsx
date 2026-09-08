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
import { Fragment } from 'react'
import { useTranslation } from 'react-i18next'

import { formatBillingCurrencyFromUSD } from '@/lib/currency'

import type { ImagePriceGroup } from '../lib/image-pricing'

export interface ImagePriceMatrixProps {
  groups: ImagePriceGroup[]
  ratio?: number
  showRechargePrice?: boolean
  priceRate?: number
  usdExchangeRate?: number
}

export function ImagePriceMatrix(props: ImagePriceMatrixProps) {
  const { t } = useTranslation()
  const ratio = props.ratio ?? 1
  const showRechargePrice = props.showRechargePrice ?? false
  const priceRate = props.priceRate ?? 1
  const usdExchangeRate = props.usdExchangeRate ?? 1
  const qualities = [
    ...new Set(
      props.groups.flatMap((group) => group.prices.map((price) => price.quality))
    ),
  ]
  if (props.groups.length === 0 || qualities.length === 0) return null

  return (
    <div
      className='grid gap-x-4 gap-y-1'
      style={{
        gridTemplateColumns: `auto repeat(${qualities.length}, minmax(0, 1fr))`,
      }}
    >
      <span />
      {qualities.map((quality) => (
        <span
          key={quality}
          className='text-muted-foreground/60 text-right text-[10px] leading-4 font-medium tracking-wider uppercase'
        >
          {t(quality)}
        </span>
      ))}
      {props.groups.map((group) => (
        <Fragment key={group.tier}>
          <span className='text-muted-foreground self-center font-mono text-[11px] leading-4 font-semibold'>
            {group.tier.toUpperCase()}
          </span>
          {qualities.map((quality) => {
            const price = group.prices.find((item) => item.quality === quality)
            return (
              <span
                key={quality}
                className='text-foreground text-right font-mono text-xs leading-4 font-semibold tabular-nums'
              >
                {price
                  ? formatBillingCurrencyFromUSD(
                      showRechargePrice
                        ? (price.priceUSD * ratio * priceRate) / usdExchangeRate
                        : price.priceUSD * ratio,
                      { digitsLarge: 4, digitsSmall: 4, abbreviate: false }
                    )
                  : '—'}
              </span>
            )
          })}
        </Fragment>
      ))}
    </div>
  )
}
