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
import { useTranslation } from 'react-i18next'

import { ToggleGroup, ToggleGroupItem } from '@/components/ui/toggle-group'
import {
  Tooltip,
  TooltipContent,
  TooltipProvider,
  TooltipTrigger,
} from '@/components/ui/tooltip'

import { isCostAccountingMode } from '../lib/mode'
import type { CostAccountingMode } from '../types'

type CostAccountingModeToggleProps = {
  mode: CostAccountingMode
  canEnableStrict: boolean
  disabled: boolean
  onChange: (mode: CostAccountingMode) => void
}

export function CostAccountingModeToggle(props: CostAccountingModeToggleProps) {
  const { t } = useTranslation()
  const modes: ReadonlyArray<{
    value: CostAccountingMode
    label: string
    description: string
  }> = [
    {
      value: 'disabled',
      label: t('Disabled'),
      description: t(
        'Turns off provider cost accounting and profit guardrails. Existing user billing continues.'
      ),
    },
    {
      value: 'tracking',
      label: t('Tracking'),
      description: t(
        'Records revenue, provider cost, profit, and anomalies without blocking missing-cost or low-margin routes.'
      ),
    },
    {
      value: 'strict',
      label: t('Strict'),
      description: t(
        'Records cost and blocks routes with unknown cost, missing rules, or expected margin below the minimum. Requires complete cost coverage.'
      ),
    },
  ]

  return (
    <TooltipProvider delay={0}>
      <ToggleGroup
        value={[props.mode]}
        onValueChange={(selection) => {
          const nextMode = selection[0]
          if (isCostAccountingMode(nextMode)) {
            props.onChange(nextMode)
          }
        }}
        disabled={props.disabled}
        aria-label={t('Cost accounting mode')}
      >
        {modes.map((option) => {
          const unavailable =
            option.value === 'strict' && !props.canEnableStrict
          return (
            <Tooltip key={option.value}>
              <TooltipTrigger
                render={
                  <ToggleGroupItem
                    value={option.value}
                    aria-disabled={unavailable}
                    onPressedChange={(_, details) => {
                      if (unavailable) {
                        details.cancel()
                      }
                    }}
                    className='aria-disabled:cursor-not-allowed aria-disabled:opacity-50 aria-disabled:hover:bg-transparent'
                  >
                    {option.label}
                  </ToggleGroupItem>
                }
              />
              <TooltipContent
                side='bottom'
                sideOffset={12}
                className='max-w-72 items-start text-pretty'
              >
                {option.description}
              </TooltipContent>
            </Tooltip>
          )
        })}
      </ToggleGroup>
    </TooltipProvider>
  )
}
