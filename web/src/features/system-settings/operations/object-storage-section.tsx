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
import { zodResolver } from '@hookform/resolvers/zod'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import {
  CloudCog,
  KeyRound,
  Link2,
  PlugZap,
  ShieldCheck,
  Trash2,
  Undo2,
} from 'lucide-react'
import { useEffect, useMemo, useState, type ReactNode } from 'react'
import { useForm, type Resolver } from 'react-hook-form'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import { z } from 'zod'

import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import {
  Form,
  FormControl,
  FormDescription,
  FormField,
  FormItem,
  FormLabel,
  FormMessage,
} from '@/components/ui/form'
import { Input } from '@/components/ui/input'
import { Switch } from '@/components/ui/switch'
import { Textarea } from '@/components/ui/textarea'
import {
  getObjectStorageSettings,
  testObjectStorageSettings,
  updateObjectStorageSettings,
} from '@/features/system-settings/api'
import type {
  ObjectStorageSettings,
  ObjectStorageSettingsRequest,
} from '@/features/system-settings/types'

import {
  SettingsControlChildren,
  SettingsControlGroup,
  SettingsForm,
  SettingsSwitchContent,
  SettingsSwitchItem,
} from '../components/settings-form-layout'
import { SettingsPageFormActions } from '../components/settings-page-context'
import { SettingsSection } from '../components/settings-section'

const objectStorageQueryKey = ['system-settings', 'object-storage'] as const

type DomainList = {
  duplicates: string[]
  invalid: string[]
  values: string[]
}

type ObjectStorageFormValues = {
  enabled: boolean
  transferMode: ObjectStorageSettings['transfer_mode']
  whitelistEnabled: boolean
  blacklistEnabled: boolean
  endpoint: string
  publicEndpoint: string
  region: string
  bucket: string
  accessKeyId: string
  secretAccessKey: string
  usePathStyle: boolean
  maxVideoSizeMb: number
  expiresSeconds: number
  transferDomainWhitelist: string
  noTransferDomainBlacklist: string
}

function normalizeDomainLine(raw: string) {
  let value = raw.trim().toLowerCase().replace(/\.+$/, '')
  const wildcard = value.startsWith('*.')
  if (wildcard) value = value.slice(2)
  if (!value.includes(']') && value.includes(':')) value = value.split(':')[0]
  return wildcard && value ? `*.${value}` : value
}

function parseDomainList(value: string): DomainList {
  const values: string[] = []
  const duplicates: string[] = []
  const invalid: string[] = []
  const seen = new Set<string>()
  const domainPattern =
    /^(?:\*\.)?(?:[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?\.)+[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?$/

  for (const raw of value.split(/\r?\n/)) {
    const normalized = normalizeDomainLine(raw)
    if (!normalized) continue
    if (!domainPattern.test(normalized)) {
      invalid.push(raw.trim())
      continue
    }
    if (seen.has(normalized)) {
      duplicates.push(normalized)
      continue
    }
    seen.add(normalized)
    values.push(normalized)
  }

  return { duplicates, invalid, values }
}

function isHttpUrl(value: string) {
  try {
    const url = new URL(value)
    return (
      (url.protocol === 'http:' || url.protocol === 'https:') &&
      Boolean(url.hostname)
    )
  } catch {
    return false
  }
}

function createObjectStorageSchema(
  t: (key: string) => string,
  secretConfigured: boolean,
  clearSecret: boolean
) {
  return z
    .object({
      enabled: z.boolean(),
      transferMode: z.enum(['default', 'all', 'rules']),
      whitelistEnabled: z.boolean(),
      blacklistEnabled: z.boolean(),
      endpoint: z.string(),
      publicEndpoint: z.string(),
      region: z.string(),
      bucket: z.string(),
      accessKeyId: z.string(),
      secretAccessKey: z.string(),
      usePathStyle: z.boolean(),
      maxVideoSizeMb: z
        .number()
        .int()
        .min(1, t('Value must be between 1 and 2048'))
        .max(2048, t('Value must be between 1 and 2048')),
      expiresSeconds: z
        .number()
        .int()
        .min(60, t('Value must be between 60 and 604800'))
        .max(604800, t('Value must be between 60 and 604800')),
      transferDomainWhitelist: z.string(),
      noTransferDomainBlacklist: z.string(),
    })
    .superRefine((values, context) => {
      if (values.enabled) {
        const requiredFields = [
          ['endpoint', values.endpoint],
          ['publicEndpoint', values.publicEndpoint],
          ['bucket', values.bucket],
          ['accessKeyId', values.accessKeyId],
        ] as const

        for (const [path, value] of requiredFields) {
          if (!value.trim()) {
            context.addIssue({
              code: 'custom',
              path: [path],
              message: t(
                'This field is required when object storage is enabled'
              ),
            })
          }
        }

        if (
          !values.secretAccessKey.trim() &&
          (!secretConfigured || clearSecret)
        ) {
          context.addIssue({
            code: 'custom',
            path: ['secretAccessKey'],
            message: t(
              'A secret access key is required when object storage is enabled'
            ),
          })
        }
      }

      for (const [path, value] of [
        ['endpoint', values.endpoint],
        ['publicEndpoint', values.publicEndpoint],
      ] as const) {
        if (value.trim() && !isHttpUrl(value.trim())) {
          context.addIssue({
            code: 'custom',
            path: [path],
            message: t('Provide a valid URL starting with http:// or https://'),
          })
        }
      }

      for (const [path, value] of [
        ['transferDomainWhitelist', values.transferDomainWhitelist],
        ['noTransferDomainBlacklist', values.noTransferDomainBlacklist],
      ] as const) {
        if (parseDomainList(value).invalid.length > 0) {
          context.addIssue({
            code: 'custom',
            path: [path],
            message: t('Enter one valid domain or wildcard domain per line'),
          })
        }
      }
    })
}

function settingsToFormValues(
  settings: ObjectStorageSettings
): ObjectStorageFormValues {
  return {
    enabled: settings.enabled,
    transferMode: settings.transfer_mode,
    whitelistEnabled: settings.whitelist_enabled,
    blacklistEnabled: settings.blacklist_enabled,
    endpoint: settings.endpoint,
    publicEndpoint: settings.public_endpoint,
    region: settings.region,
    bucket: settings.bucket,
    accessKeyId: settings.access_key_id,
    secretAccessKey: '',
    usePathStyle: settings.use_path_style,
    maxVideoSizeMb: settings.max_video_size_mb,
    expiresSeconds: settings.expires_seconds,
    transferDomainWhitelist: settings.transfer_domain_whitelist.join('\n'),
    noTransferDomainBlacklist: settings.no_transfer_domain_blacklist.join('\n'),
  }
}

function formValuesToRequest(
  values: ObjectStorageFormValues,
  clearSecret: boolean
): ObjectStorageSettingsRequest {
  return {
    enabled: values.enabled,
    transfer_mode: values.transferMode,
    whitelist_enabled: values.whitelistEnabled,
    blacklist_enabled: values.blacklistEnabled,
    endpoint: values.endpoint.trim(),
    public_endpoint: values.publicEndpoint.trim(),
    region: values.region.trim(),
    bucket: values.bucket.trim(),
    access_key_id: values.accessKeyId.trim(),
    secret_access_key: values.secretAccessKey.trim(),
    clear_secret: clearSecret,
    use_path_style: values.usePathStyle,
    max_video_size_mb: values.maxVideoSizeMb,
    expires_seconds: values.expiresSeconds,
    transfer_domain_whitelist: parseDomainList(values.transferDomainWhitelist)
      .values,
    no_transfer_domain_blacklist: parseDomainList(
      values.noTransferDomainBlacklist
    ).values,
  }
}

function FormGroup(props: {
  children: ReactNode
  icon: ReactNode
  title: string
  description?: string
}) {
  return (
    <div
      data-settings-form-span='full'
      className='space-y-4 border-t pt-5 first:border-t-0 first:pt-0'
    >
      <div className='flex min-w-0 items-start gap-2.5'>
        <span
          className='text-muted-foreground mt-0.5 [&>svg]:size-4'
          aria-hidden='true'
        >
          {props.icon}
        </span>
        <div className='min-w-0 space-y-0.5'>
          <h4 className='text-sm font-medium'>{props.title}</h4>
          {props.description && (
            <p className='text-muted-foreground text-sm'>{props.description}</p>
          )}
        </div>
      </div>
      <div className='grid min-w-0 gap-x-5 gap-y-5 lg:grid-cols-2 [&>[data-slot=form-item]]:min-w-0 lg:[&>[data-slot=form-item]:has([data-slot=switch])]:col-span-2 lg:[&>[data-slot=form-item]:has(textarea)]:col-span-2'>
        {props.children}
      </div>
    </div>
  )
}

export function ObjectStorageSection() {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const [secretConfigured, setSecretConfigured] = useState(false)
  const [clearSecret, setClearSecret] = useState(false)

  const settingsQuery = useQuery({
    queryKey: objectStorageQueryKey,
    queryFn: getObjectStorageSettings,
  })
  const schema = useMemo(
    () => createObjectStorageSchema(t, secretConfigured, clearSecret),
    [clearSecret, secretConfigured, t]
  )
  const form = useForm<ObjectStorageFormValues>({
    resolver: zodResolver(schema) as Resolver<ObjectStorageFormValues>,
    defaultValues: {
      enabled: false,
      transferMode: 'default',
      whitelistEnabled: false,
      blacklistEnabled: false,
      endpoint: '',
      publicEndpoint: '',
      region: 'us-east-1',
      bucket: '',
      accessKeyId: '',
      secretAccessKey: '',
      usePathStyle: false,
      maxVideoSizeMb: 512,
      expiresSeconds: 86400,
      transferDomainWhitelist: '',
      noTransferDomainBlacklist: '',
    },
  })

  useEffect(() => {
    const settings = settingsQuery.data?.data
    if (!settings) return
    form.reset(settingsToFormValues(settings))
    setSecretConfigured(settings.secret_configured)
    setClearSecret(false)
  }, [form, settingsQuery.data])

  const whitelist = form.watch('transferDomainWhitelist')
  const blacklist = form.watch('noTransferDomainBlacklist')
  const enabled = form.watch('enabled')
  const transferMode = form.watch('transferMode')
  const whitelistEnabled = form.watch('whitelistEnabled')
  const blacklistEnabled = form.watch('blacklistEnabled')
  const whitelistInfo = useMemo(() => parseDomainList(whitelist), [whitelist])
  const blacklistInfo = useMemo(() => parseDomainList(blacklist), [blacklist])
  const conflicts = useMemo(() => {
    const blacklistValues = new Set(blacklistInfo.values)
    return whitelistInfo.values.filter((domain) => blacklistValues.has(domain))
  }, [blacklistInfo.values, whitelistInfo.values])

  const saveMutation = useMutation({
    mutationFn: updateObjectStorageSettings,
    onSuccess: async (response) => {
      setSecretConfigured(response.data.secret_configured)
      setClearSecret(false)
      form.reset(settingsToFormValues(response.data))
      queryClient.setQueryData(objectStorageQueryKey, response)
      toast.success(t('Object storage settings saved'))
    },
    onError: () => toast.error(t('Failed to save object storage settings')),
  })
  const testMutation = useMutation({
    mutationFn: testObjectStorageSettings,
    onSuccess: () => toast.success(t('Object storage connection succeeded')),
    onError: () => toast.error(t('Object storage connection failed')),
  })

  const onSave = form.handleSubmit(async (values) => {
    await saveMutation.mutateAsync(formValuesToRequest(values, clearSecret))
  })
  const onTest = form.handleSubmit(async (values) => {
    await testMutation.mutateAsync(formValuesToRequest(values, clearSecret))
  })
  const isBusy = saveMutation.isPending || testMutation.isPending

  if (settingsQuery.isPending) {
    return (
      <SettingsSection title={t('Object Storage')}>
        <p className='text-muted-foreground text-sm'>
          {t('Loading object storage settings...')}
        </p>
      </SettingsSection>
    )
  }

  if (settingsQuery.isError || !settingsQuery.data?.data) {
    return (
      <SettingsSection title={t('Object Storage')}>
        <Alert variant='destructive'>
          <AlertTitle>{t('Failed to load object storage settings')}</AlertTitle>
          <AlertDescription>
            <Button
              type='button'
              variant='outline'
              size='sm'
              className='mt-2'
              onClick={() => void settingsQuery.refetch()}
            >
              {t('Retry')}
            </Button>
          </AlertDescription>
        </Alert>
      </SettingsSection>
    )
  }

  return (
    <SettingsSection title={t('Object Storage')}>
      <Form {...form}>
        <SettingsForm onSubmit={onSave} autoComplete='off'>
          <SettingsPageFormActions
            onSave={onSave}
            isSaving={saveMutation.isPending}
            isSaveDisabled={!form.formState.isDirty && !clearSecret}
            saveLabel='Save object storage settings'
          />

          <FormGroup
            icon={<CloudCog />}
            title={t('Enable object storage')}
            description={t(
              'Transfer selected upstream video results before returning them to customers.'
            )}
          >
            <FormField
              control={form.control}
              name='enabled'
              render={({ field }) => (
                <SettingsSwitchItem>
                  <SettingsSwitchContent>
                    <FormLabel>{t('Enable video result transfer')}</FormLabel>
                    <FormDescription>
                      {t(
                        'Turn off to return upstream video links without object storage transfer.'
                      )}
                    </FormDescription>
                  </SettingsSwitchContent>
                  <FormControl>
                    <Switch
                      aria-label={t('Enable video result transfer')}
                      checked={field.value}
                      onCheckedChange={field.onChange}
                      disabled={isBusy}
                    />
                  </FormControl>
                </SettingsSwitchItem>
              )}
            />

            <SettingsControlGroup>
              <FormField
                control={form.control}
                name='transferMode'
                render={({ field }) => (
                  <SettingsSwitchItem className='py-2'>
                    <SettingsSwitchContent>
                      <FormLabel>{t('Enable all video transfer')}</FormLabel>
                      <FormDescription>
                        {t('All eligible videos are uploaded.')}
                      </FormDescription>
                    </SettingsSwitchContent>
                    <FormControl>
                      <Switch
                        aria-label={t('Enable all video transfer')}
                        checked={field.value === 'all'}
                        onCheckedChange={(checked) =>
                          field.onChange(checked ? 'all' : 'default')
                        }
                        disabled={isBusy}
                      />
                    </FormControl>
                  </SettingsSwitchItem>
                )}
              />
              <FormField
                control={form.control}
                name='transferMode'
                render={({ field }) => (
                  <SettingsSwitchItem className='py-2'>
                    <SettingsSwitchContent>
                      <FormLabel>{t('Enable domain rules')}</FormLabel>
                      <FormDescription>
                        {t('Use whitelist and blacklist rules to decide.')}
                      </FormDescription>
                    </SettingsSwitchContent>
                    <FormControl>
                      <Switch
                        aria-label={t('Enable domain rules')}
                        checked={field.value === 'rules'}
                        onCheckedChange={(checked) =>
                          field.onChange(checked ? 'rules' : 'default')
                        }
                        disabled={isBusy}
                      />
                    </FormControl>
                  </SettingsSwitchItem>
                )}
              />
            </SettingsControlGroup>

            {!enabled && (
              <Alert className='lg:col-span-2'>
                <AlertTitle>
                  {t('Transfer is paused while the main switch is off.')}
                </AlertTitle>
              </Alert>
            )}
          </FormGroup>

          <FormGroup
            icon={<PlugZap />}
            title={t('Connection')}
            description={t(
              'Configure the S3-compatible API and download endpoints.'
            )}
          >
            <FormField
              control={form.control}
              name='endpoint'
              render={({ field }) => (
                <FormItem>
                  <FormLabel>{t('Endpoint')}</FormLabel>
                  <FormControl>
                    <Input
                      type='url'
                      inputMode='url'
                      aria-label={t('Endpoint')}
                      placeholder='https://s3.example.com'
                      {...field}
                    />
                  </FormControl>
                  <FormDescription>
                    {t('Used for object upload, verification, and deletion.')}
                  </FormDescription>
                  <FormMessage />
                </FormItem>
              )}
            />
            <FormField
              control={form.control}
              name='publicEndpoint'
              render={({ field }) => (
                <FormItem>
                  <FormLabel>{t('Public endpoint')}</FormLabel>
                  <FormControl>
                    <Input
                      type='url'
                      inputMode='url'
                      aria-label={t('Public endpoint')}
                      placeholder='https://download.example.com'
                      {...field}
                    />
                  </FormControl>
                  <FormDescription>
                    {t('Used to generate signed video download links.')}
                  </FormDescription>
                  <FormMessage />
                </FormItem>
              )}
            />
            <FormField
              control={form.control}
              name='region'
              render={({ field }) => (
                <FormItem>
                  <FormLabel>{t('Region')}</FormLabel>
                  <FormControl>
                    <Input
                      aria-label={t('Region')}
                      placeholder='us-east-1'
                      {...field}
                    />
                  </FormControl>
                  <FormDescription>
                    {t('Defaults to us-east-1 when left blank.')}
                  </FormDescription>
                  <FormMessage />
                </FormItem>
              )}
            />
            <FormField
              control={form.control}
              name='bucket'
              render={({ field }) => (
                <FormItem>
                  <FormLabel>{t('Bucket')}</FormLabel>
                  <FormControl>
                    <Input
                      aria-label={t('Bucket')}
                      autoComplete='off'
                      {...field}
                    />
                  </FormControl>
                  <FormMessage />
                </FormItem>
              )}
            />
            <FormField
              control={form.control}
              name='usePathStyle'
              render={({ field }) => (
                <SettingsSwitchItem>
                  <SettingsSwitchContent>
                    <FormLabel>{t('Use path-style addressing')}</FormLabel>
                    <FormDescription>
                      {t(
                        'Enable for providers such as MinIO that require the bucket in the URL path.'
                      )}
                    </FormDescription>
                  </SettingsSwitchContent>
                  <FormControl>
                    <Switch
                      checked={field.value}
                      onCheckedChange={field.onChange}
                      disabled={isBusy}
                    />
                  </FormControl>
                </SettingsSwitchItem>
              )}
            />
          </FormGroup>

          <FormGroup
            icon={<KeyRound />}
            title={t('Credentials')}
            description={t(
              'Credentials are only sent to the server and are never displayed again.'
            )}
          >
            <FormField
              control={form.control}
              name='accessKeyId'
              render={({ field }) => (
                <FormItem>
                  <FormLabel>{t('Access Key ID')}</FormLabel>
                  <FormControl>
                    <Input
                      aria-label={t('Access Key ID')}
                      autoComplete='off'
                      {...field}
                    />
                  </FormControl>
                  <FormMessage />
                </FormItem>
              )}
            />
            <FormField
              control={form.control}
              name='secretAccessKey'
              render={({ field }) => (
                <FormItem>
                  <div className='flex flex-wrap items-center justify-between gap-2'>
                    <FormLabel>{t('Secret Access Key')}</FormLabel>
                    <Badge
                      variant={
                        secretConfigured && !clearSecret
                          ? 'secondary'
                          : 'outline'
                      }
                    >
                      {secretConfigured && !clearSecret
                        ? t('Secret configured')
                        : t('No stored secret')}
                    </Badge>
                  </div>
                  <FormControl>
                    <Input
                      type='password'
                      aria-label={t('Secret Access Key')}
                      autoComplete='new-password'
                      placeholder={t('Leave blank to keep the stored secret')}
                      {...field}
                      onChange={(event) => {
                        field.onChange(event)
                        if (event.target.value) setClearSecret(false)
                      }}
                    />
                  </FormControl>
                  <FormDescription>
                    {t('Enter a new value to replace the stored secret.')}
                  </FormDescription>
                  <FormMessage />
                  <Button
                    type='button'
                    variant='outline'
                    size='sm'
                    onClick={() => {
                      if (clearSecret) {
                        setClearSecret(false)
                        return
                      }
                      form.setValue('secretAccessKey', '', {
                        shouldDirty: true,
                      })
                      setClearSecret(true)
                    }}
                    disabled={isBusy || (!secretConfigured && !clearSecret)}
                  >
                    {clearSecret ? (
                      <Undo2 data-icon='inline-start' />
                    ) : (
                      <Trash2 data-icon='inline-start' />
                    )}
                    {clearSecret
                      ? t('Keep stored secret')
                      : t('Clear stored secret')}
                  </Button>
                </FormItem>
              )}
            />
            {clearSecret && (
              <Alert variant='destructive' className='lg:col-span-2'>
                <AlertTitle>
                  {t('The stored secret will be cleared when you save.')}
                </AlertTitle>
              </Alert>
            )}
          </FormGroup>

          <FormGroup
            icon={<Link2 />}
            title={t('Link and limits')}
            description={t(
              'Control signed link lifetime and the largest video accepted for transfer.'
            )}
          >
            <FormField
              control={form.control}
              name='expiresSeconds'
              render={({ field }) => (
                <FormItem>
                  <FormLabel>{t('Signed link lifetime (seconds)')}</FormLabel>
                  <FormControl>
                    <Input
                      type='number'
                      min={60}
                      max={604800}
                      aria-label={t('Signed link lifetime (seconds)')}
                      {...field}
                      onChange={(event) =>
                        field.onChange(event.target.valueAsNumber)
                      }
                    />
                  </FormControl>
                  <FormDescription>
                    {t('Allowed range: 60 to 604800 seconds. Default: 86400.')}
                  </FormDescription>
                  <FormMessage />
                </FormItem>
              )}
            />
            <FormField
              control={form.control}
              name='maxVideoSizeMb'
              render={({ field }) => (
                <FormItem>
                  <FormLabel>{t('Maximum video size (MB)')}</FormLabel>
                  <FormControl>
                    <Input
                      type='number'
                      min={1}
                      max={2048}
                      aria-label={t('Maximum video size (MB)')}
                      {...field}
                      onChange={(event) =>
                        field.onChange(event.target.valueAsNumber)
                      }
                    />
                  </FormControl>
                  <FormDescription>
                    {t('Allowed range: 1 to 2048 MB. Default: 512.')}
                  </FormDescription>
                  <FormMessage />
                </FormItem>
              )}
            />
          </FormGroup>

          {transferMode === 'rules' && (
            <FormGroup
              icon={<ShieldCheck />}
              title={t('Domain rules')}
              description={t(
                'Domains not selected by an enabled rule are not transferred.'
              )}
            >
              <SettingsControlGroup>
                <FormField
                  control={form.control}
                  name='whitelistEnabled'
                  render={({ field }) => (
                    <SettingsSwitchItem className='py-2'>
                      <SettingsSwitchContent>
                        <FormLabel>{t('Enable transfer whitelist')}</FormLabel>
                        <FormDescription>
                          {t(
                            'Videos from these domains must be transferred to object storage.'
                          )}
                        </FormDescription>
                      </SettingsSwitchContent>
                      <FormControl>
                        <Switch
                          aria-label={t('Enable transfer whitelist')}
                          checked={field.value}
                          onCheckedChange={field.onChange}
                          disabled={isBusy}
                        />
                      </FormControl>
                    </SettingsSwitchItem>
                  )}
                />
                <SettingsControlChildren>
                  <FormField
                    control={form.control}
                    name='transferDomainWhitelist'
                    render={({ field }) => (
                      <FormItem>
                        <FormLabel>{t('Transfer domain whitelist')}</FormLabel>
                        <FormControl>
                          <Textarea
                            aria-label={t('Transfer domain whitelist')}
                            rows={5}
                            placeholder={
                              'provider.example.com\n*.media.example.com'
                            }
                            {...field}
                            disabled={!whitelistEnabled || isBusy}
                          />
                        </FormControl>
                        <FormMessage />
                      </FormItem>
                    )}
                  />
                </SettingsControlChildren>
              </SettingsControlGroup>

              <SettingsControlGroup>
                <FormField
                  control={form.control}
                  name='blacklistEnabled'
                  render={({ field }) => (
                    <SettingsSwitchItem className='py-2'>
                      <SettingsSwitchContent>
                        <FormLabel>{t('Enable transfer blacklist')}</FormLabel>
                        <FormDescription>
                          {t(
                            'Videos from these domains are returned without object storage transfer.'
                          )}
                        </FormDescription>
                      </SettingsSwitchContent>
                      <FormControl>
                        <Switch
                          aria-label={t('Enable transfer blacklist')}
                          checked={field.value}
                          onCheckedChange={field.onChange}
                          disabled={isBusy}
                        />
                      </FormControl>
                    </SettingsSwitchItem>
                  )}
                />
                <SettingsControlChildren>
                  <FormField
                    control={form.control}
                    name='noTransferDomainBlacklist'
                    render={({ field }) => (
                      <FormItem>
                        <FormLabel>
                          {t('No-transfer domain blacklist')}
                        </FormLabel>
                        <FormControl>
                          <Textarea
                            aria-label={t('No-transfer domain blacklist')}
                            rows={5}
                            placeholder={
                              'official.example.com\n*.trusted.example.com'
                            }
                            {...field}
                            disabled={!blacklistEnabled || isBusy}
                          />
                        </FormControl>
                        <FormMessage />
                      </FormItem>
                    )}
                  />
                </SettingsControlChildren>
              </SettingsControlGroup>

              {(whitelistInfo.duplicates.length > 0 ||
                blacklistInfo.duplicates.length > 0) && (
                <Alert className='lg:col-span-2'>
                  <AlertTitle>
                    {t('Duplicate domains will be removed when saved.')}
                  </AlertTitle>
                </Alert>
              )}
              {whitelistEnabled && blacklistEnabled && (
                <Alert className='lg:col-span-2'>
                  <AlertTitle>
                    {t('The blacklist takes priority when both rules match.')}
                  </AlertTitle>
                  {conflicts.length > 0 && (
                    <AlertDescription>
                      {t('Some domains appear in both lists.')}
                    </AlertDescription>
                  )}
                </Alert>
              )}
            </FormGroup>
          )}

          <div
            data-settings-form-span='full'
            className='flex flex-wrap items-center justify-between gap-3 border-t pt-5'
          >
            <div className='min-w-0'>
              <p className='text-sm font-medium'>{t('Connection test')}</p>
              <p className='text-muted-foreground text-sm'>
                {t(
                  'Writes and deletes a small probe object without saving these settings.'
                )}
              </p>
            </div>
            <Button
              type='button'
              variant='outline'
              onClick={onTest}
              disabled={isBusy}
            >
              <PlugZap data-icon='inline-start' />
              {testMutation.isPending
                ? t('Testing connection...')
                : t('Test connection')}
            </Button>
          </div>
        </SettingsForm>
      </Form>
    </SettingsSection>
  )
}
