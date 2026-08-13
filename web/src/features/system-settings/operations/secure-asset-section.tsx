/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as published by
the Free Software Foundation, either version 3 of the License, or
(at your option) any later version.
*/
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { Images } from 'lucide-react'
import { useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { Label } from '@/components/ui/label'
import {
  getSecureAssetSettings,
  updateSecureAssetSettings,
} from '@/features/system-settings/api'

import { SettingsForm } from '../components/settings-form-layout'
import { SettingsPageFormActions } from '../components/settings-page-context'
import { SettingsSection } from '../components/settings-section'

const secureAssetQueryKey = ['system-settings', 'secure-assets'] as const

export function SecureAssetSection() {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const settingsQuery = useQuery({
    queryKey: secureAssetQueryKey,
    queryFn: getSecureAssetSettings,
  })
  const [channelID, setChannelID] = useState<number>(0)

  useEffect(() => {
    if (settingsQuery.data?.data) {
      setChannelID(settingsQuery.data.data.default_channel_id)
    }
  }, [settingsQuery.data])

  const saveMutation = useMutation({
    mutationFn: updateSecureAssetSettings,
    onSuccess: (response) => {
      queryClient.setQueryData(secureAssetQueryKey, response)
      setChannelID(response.data.default_channel_id)
      toast.success(t('Secure asset settings saved'))
    },
    onError: () => toast.error(t('Failed to save Secure asset settings')),
  })

  if (settingsQuery.isPending) {
    return (
      <SettingsSection title={t('Secure role assets')}>
        <p className='text-muted-foreground text-sm'>
          {t('Loading Secure asset settings...')}
        </p>
      </SettingsSection>
    )
  }
  if (settingsQuery.isError || !settingsQuery.data?.data) {
    return (
      <SettingsSection title={t('Secure role assets')}>
        <p className='text-destructive text-sm'>
          {t('Failed to load Secure asset settings')}
        </p>
      </SettingsSection>
    )
  }

  const channels = settingsQuery.data.data.channels
  return (
    <SettingsSection title={t('Secure role assets')}>
      <SettingsForm onSubmit={(event) => { event.preventDefault(); saveMutation.mutate(channelID) }}>
        <SettingsPageFormActions
          onSave={() => saveMutation.mutate(channelID)}
          isSaving={saveMutation.isPending}
          isSaveDisabled={channelID <= 0 || !settingsQuery.data.data.channels.some((channel) => channel.id === channelID)}
          saveLabel='Save Secure asset settings'
        />
        <div className='flex items-start gap-3 rounded-lg border p-4'>
          <Images className='text-muted-foreground mt-0.5 size-4' aria-hidden='true' />
          <div className='min-w-0 flex-1 space-y-2'>
            <Label htmlFor='secure-asset-channel'>{t('Default Secure enterprise channel')}</Label>
            <select
              id='secure-asset-channel'
              aria-label={t('Secure asset channel')}
              className='border-input bg-background h-9 w-full rounded-md border px-3 text-sm'
              value={channelID > 0 ? String(channelID) : ''}
              onChange={(event) => setChannelID(Number(event.target.value))}
              disabled={saveMutation.isPending}
            >
              <option value=''>{t('Select a Secure enterprise channel')}</option>
              {channels.map((channel) => (
                <option key={channel.id} value={channel.id}>
                  {channel.name || `#${channel.id}`}
                </option>
              ))}
            </select>
            <p className='text-muted-foreground text-xs'>
              {t('Role assets use the channel and API key selected when they are created.')}
            </p>
          </div>
        </div>
      </SettingsForm>
    </SettingsSection>
  )
}
