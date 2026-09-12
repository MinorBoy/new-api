import { useTranslation } from 'react-i18next'

import { cn } from '@/lib/utils'

import { StatusBadge, type StatusBadgeProps } from './status-badge'

type GroupBadgeProps = Omit<
  StatusBadgeProps,
  'autoColor' | 'label' | 'variant'
> & {
  group?: string | null
  label?: string
  ratio?: number | null
}

function getGroupRatioClassName(ratio: number): string {
  if (ratio > 1) {
    return 'bg-warning/10 text-warning'
  }
  if (ratio < 1) {
    return 'bg-info/10 text-info'
  }
  return 'bg-muted text-muted-foreground'
}

function getGroupLabel(params: {
  labelOverride?: string
  groupName?: string
  isAutoGroup: boolean
  isEmptyGroup: boolean
  t: (key: string) => string
}): string {
  if (params.labelOverride) return params.labelOverride
  if (params.isEmptyGroup) return params.t('User Group')
  if (params.isAutoGroup) return params.t('Auto')
  return params.groupName ?? ''
}

export function GroupBadge(props: GroupBadgeProps) {
  const { t } = useTranslation()
  const {
    group,
    label: labelOverride,
    ratio,
    copyable = false,
    showDot,
    className,
    ...badgeProps
  } = props
  const groupName = group?.trim()
  const isAutoGroup = groupName === 'auto'
  const isEmptyGroup = !groupName
  const isSpecialGroup = isAutoGroup || isEmptyGroup
  const label = getGroupLabel({
    labelOverride,
    groupName,
    isAutoGroup,
    isEmptyGroup,
    t,
  })

  const badge = (
    <StatusBadge
      {...badgeProps}
      copyable={copyable}
      label={label}
      showDot={showDot ?? (isSpecialGroup ? false : undefined)}
      variant={isSpecialGroup ? 'neutral' : undefined}
      autoColor={isSpecialGroup ? undefined : groupName}
      className={cn('min-w-0 shrink overflow-hidden', className)}
    />
  )

  if (ratio == null) {
    return badge
  }

  return (
    <span className='inline-flex max-w-full min-w-0 items-center gap-2 text-xs'>
      <span className='max-w-full min-w-0 overflow-hidden'>{badge}</span>
      <span
        className={cn(
          'inline-flex h-5 shrink-0 items-center rounded-full px-1.5 font-mono text-xs leading-none font-medium tabular-nums',
          getGroupRatioClassName(ratio)
        )}
      >
        <span>{ratio}x</span>
      </span>
    </span>
  )
}
