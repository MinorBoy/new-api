import { Check, ChevronDown } from 'lucide-react'
import { useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { GroupBadge } from '@/components/group-badge'
import { Button } from '@/components/ui/button'
import {
  Command,
  CommandEmpty,
  CommandGroup,
  CommandInput,
  CommandItem,
  CommandList,
} from '@/components/ui/command'
import {
  Popover,
  PopoverContent,
  PopoverTrigger,
} from '@/components/ui/popover'
import { cn } from '@/lib/utils'

import { updateApiKeyGroup } from '../api'
import { ERROR_MESSAGES } from '../constants'
import type { ApiKey } from '../types'
import type { ApiKeyGroupOption } from './api-key-group-combobox'
import { useApiKeys } from './api-keys-provider'

type ApiKeyGroupCellProps = {
  apiKey: ApiKey
  groupRatios: Record<string, number>
}

export function ApiKeyGroupCell({ apiKey, groupRatios }: ApiKeyGroupCellProps) {
  const { t } = useTranslation()
  const { triggerRefresh } = useApiKeys()
  const [open, setOpen] = useState(false)
  const [searchValue, setSearchValue] = useState('')
  const [submitting, setSubmitting] = useState(false)

  // Build options the same way the create/update drawer does.
  const options: ApiKeyGroupOption[] = useMemo(
    () =>
      Object.entries(groupRatios).map(([key, ratio]) => ({
        value: key,
        label: key,
        desc: key,
        ratio,
      })),
    [groupRatios]
  )

  const filteredOptions = useMemo(() => {
    const search = searchValue.trim().toLowerCase()
    if (!search) return options
    return options.filter(
      (option) =>
        option.value.toLowerCase().includes(search) ||
        option.label.toLowerCase().includes(search)
    )
  }, [options, searchValue])

  const currentGroup = apiKey.group ?? ''

  const handleSelect = async (selectedValue: string) => {
    if (selectedValue === currentGroup || submitting) {
      setOpen(false)
      setSearchValue('')
      return
    }
    setSubmitting(true)
    try {
      // Preserve the existing cross_group_retry toggle when switching groups;
      // it is only meaningful for the "auto" group.
      const crossGroupRetry =
        selectedValue === 'auto' ? apiKey.cross_group_retry : false
      const result = await updateApiKeyGroup(
        apiKey.id,
        selectedValue,
        crossGroupRetry
      )
      if (result.success) {
        toast.success(t('Group updated'))
        triggerRefresh()
      } else {
        toast.error(result.message || t(ERROR_MESSAGES.UPDATE_FAILED))
      }
    } catch {
      toast.error(t(ERROR_MESSAGES.UNEXPECTED))
    } finally {
      setSubmitting(false)
      setOpen(false)
      setSearchValue('')
    }
  }

  return (
    <Popover open={open} onOpenChange={setOpen}>
      <PopoverTrigger
        render={
          <Button
            type='button'
            variant='ghost'
            role='combobox'
            aria-expanded={open}
            disabled={submitting}
            aria-label={t('Change group')}
            className='hover:bg-muted/50 focus-visible:ring-ring/20 -ml-1.5 h-auto min-h-9 w-full justify-start gap-1 rounded-md px-1.5 py-1 text-xs shadow-none transition-colors focus-visible:ring-[3px]'
          />
        }
      >
        <span className='inline-flex min-w-0 items-center gap-1'>
          <GroupBadge
            group={currentGroup}
            ratio={
              currentGroup && currentGroup !== 'auto'
                ? groupRatios[currentGroup]
                : undefined
            }
          />
          {apiKey.cross_group_retry && currentGroup === 'auto' && (
            <span className='text-muted-foreground text-[10px]'>
              {t('Cross-group')}
            </span>
          )}
        </span>
        <ChevronDown className='ml-auto h-3.5 w-3.5 shrink-0 opacity-50' />
      </PopoverTrigger>
      <PopoverContent
        className='w-[var(--anchor-width)] min-w-[200px] overflow-hidden rounded-xl p-0 shadow-lg'
        align='start'
        onWheel={(event) => event.stopPropagation()}
        onTouchMove={(event) => event.stopPropagation()}
        onPointerDown={(event) => event.stopPropagation()}
      >
        <Command shouldFilter={false}>
          <CommandInput
            placeholder={t('Search...')}
            value={searchValue}
            onValueChange={setSearchValue}
          />
          <CommandList className='max-h-[300px]'>
            <CommandEmpty>{t('No group found.')}</CommandEmpty>
            <CommandGroup>
              {filteredOptions.map((option) => {
                const ratio =
                  typeof option.ratio === 'number' ? option.ratio : undefined
                return (
                  <CommandItem
                    key={option.value}
                    value={option.value}
                    onSelect={() => handleSelect(option.value)}
                    className='gap-2 rounded-lg px-2 py-1.5'
                  >
                    <Check
                      className={cn(
                        'h-3.5 w-3.5 shrink-0',
                        currentGroup === option.value
                          ? 'opacity-100'
                          : 'opacity-0'
                      )}
                    />
                    <span className='min-w-0 flex-1'>
                      <GroupBadge group={option.value} ratio={ratio} />
                    </span>
                  </CommandItem>
                )
              })}
            </CommandGroup>
          </CommandList>
        </Command>
      </PopoverContent>
    </Popover>
  )
}
