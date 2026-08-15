import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { Copy, Images, KeyRound, RefreshCw } from 'lucide-react'
import { useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { CopyButton } from '@/components/copy-button'
import { SectionPageLayout } from '@/components/layout'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardHeader } from '@/components/ui/card'
import {
  Dialog,
  DialogContent,
  DialogTitle,
  DialogTrigger,
} from '@/components/ui/dialog'
import { FieldDescription, FieldLabel } from '@/components/ui/field'
import { Input } from '@/components/ui/input'
import { NativeSelect, NativeSelectOption } from '@/components/ui/native-select'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'

import { getApiKeyValue, getEnabledApiKeys } from '../keys/api'
import type { ApiKey } from '../keys/types'
import { createAsset, listAssets, refreshAsset } from './api'
import type { Asset } from './types'

const assetQueryKey = (tokenID: string) => ['role-assets', tokenID] as const

async function loadAssets(apiKey: string) {
  const response = await listAssets(apiKey)
  const processing = response.data.items.filter(
    (asset) => asset.status === 'processing' || asset.status === 'pending'
  )
  if (processing.length === 0) return response
  await Promise.allSettled(
    processing.map((asset) => refreshAsset(apiKey, asset))
  )
  return listAssets(apiKey)
}

function apiKeyLabel(apiKey: ApiKey): string {
  return `${apiKey.name} · ${apiKey.group || 'default'} · ${apiKey.key}`
}

export function Assets() {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const [url, setURL] = useState('')
  const [selectedKeyId, setSelectedKeyId] = useState('')
  const apiKeysQuery = useQuery({
    queryKey: ['assets', 'api-keys'],
    queryFn: getEnabledApiKeys,
    staleTime: 30_000,
  })
  const apiKeys = apiKeysQuery.data ?? []
  const selectedKey = apiKeys.find(
    (apiKey) => String(apiKey.id) === selectedKeyId
  )
  const apiKeyValueQuery = useQuery({
    queryKey: ['assets', 'api-key-value', selectedKeyId],
    queryFn: () => getApiKeyValue(Number(selectedKeyId)),
    enabled: Boolean(selectedKey),
    staleTime: 30_000,
  })
  const selectedApiKey = selectedKey ? (apiKeyValueQuery.data ?? '') : ''
  const assetsQuery = useQuery({
    queryKey: assetQueryKey(selectedKeyId),
    queryFn: () => loadAssets(selectedApiKey),
    enabled: Boolean(selectedApiKey),
    refetchInterval: (query) =>
      query.state.data?.data.items.some(
        (asset) => asset.status === 'processing' || asset.status === 'pending'
      )
        ? 2000
        : false,
  })
  const createMutation = useMutation({
    mutationFn: (input: { apiKey: string; tokenID: string; url: string }) =>
      createAsset(input.apiKey, input.url),
    onSuccess: (_response, input) => {
      setURL('')
      void queryClient.invalidateQueries({
        queryKey: assetQueryKey(input.tokenID),
      })
      toast.success(t('Asset created'))
    },
    onError: () => toast.error(t('Failed to create asset')),
  })
  const refreshMutation = useMutation({
    mutationFn: (input: { apiKey: string; tokenID: string; asset: Asset }) =>
      refreshAsset(input.apiKey, input.asset),
    onSuccess: (_response, input) =>
      void queryClient.invalidateQueries({
        queryKey: assetQueryKey(input.tokenID),
      }),
  })
  const items = assetsQuery.data?.data.items ?? []

  let apiKeyPlaceholder = t('Select an API key')
  if (apiKeysQuery.isLoading) {
    apiKeyPlaceholder = t('Loading API keys...')
  } else if (apiKeys.length === 0) {
    apiKeyPlaceholder = t('No enabled API keys')
  }

  let emptyMessage = ''
  let emptyMessageIsError = false
  if (!selectedKeyId) {
    emptyMessage = t('Select an API key to view assets.')
  } else if (apiKeyValueQuery.isError) {
    emptyMessage = t('Failed to load API key')
    emptyMessageIsError = true
  } else if (items.length === 0 && !assetsQuery.isLoading) {
    emptyMessage = t('No assets yet')
  }

  const statusVariant = (status: Asset['status']) => {
    if (status === 'active') return 'default' as const
    if (status === 'failed') return 'destructive' as const
    return 'outline' as const
  }

  useEffect(() => {
    if (assetsQuery.error) toast.error(t('Failed to load assets'))
  }, [assetsQuery.error, t])

  useEffect(() => {
    if (apiKeysQuery.error) toast.error(t('Failed to load API keys'))
  }, [apiKeysQuery.error, t])

  useEffect(() => {
    if (apiKeyValueQuery.error) toast.error(t('Failed to load API key'))
  }, [apiKeyValueQuery.error, t])

  const copyReference = async (asset: Asset) => {
    if (!asset.reference) return
    await navigator.clipboard.writeText(asset.reference)
    toast.success(t('Copied to clipboard'))
  }

  return (
    <SectionPageLayout>
      <SectionPageLayout.Title>
        <span className='flex items-center gap-2'>
          <Images className='size-4' aria-hidden='true' />
          {t('Asset library')}
        </span>
      </SectionPageLayout.Title>
      <SectionPageLayout.Content>
        <div className='mx-auto flex w-full max-w-6xl flex-col gap-4'>
          <Card>
            <CardHeader>
              <h2 className='flex items-center gap-2 text-base font-medium'>
                <KeyRound className='size-4' aria-hidden='true' />
                {t('API key')}
              </h2>
            </CardHeader>
            <CardContent className='space-y-2'>
              <FieldLabel htmlFor='asset-api-key'>{t('API key')}</FieldLabel>
              <NativeSelect
                id='asset-api-key'
                aria-label={t('API key')}
                className='w-full sm:max-w-xl'
                disabled={apiKeysQuery.isLoading || createMutation.isPending}
                value={selectedKeyId}
                onChange={(event) => setSelectedKeyId(event.target.value)}
              >
                <NativeSelectOption value=''>
                  {apiKeyPlaceholder}
                </NativeSelectOption>
                {apiKeys.map((apiKey) => (
                  <NativeSelectOption key={apiKey.id} value={String(apiKey.id)}>
                    {apiKeyLabel(apiKey)}
                  </NativeSelectOption>
                ))}
              </NativeSelect>
              <FieldDescription>
                {t(
                  'Choose an enabled API key before creating or viewing assets.'
                )}
              </FieldDescription>
            </CardContent>
          </Card>
          <Card>
            <CardHeader>
              <h2 className='text-base font-medium'>
                {t('Create image asset')}
              </h2>
            </CardHeader>
            <CardContent>
              <form
                className='flex flex-col gap-2 sm:flex-row'
                onSubmit={(event) => {
                  event.preventDefault()
                  if (!selectedApiKey || !url.trim()) return
                  createMutation.mutate({
                    apiKey: selectedApiKey,
                    tokenID: selectedKeyId,
                    url: url.trim(),
                  })
                }}
              >
                <Input
                  aria-label={t('Public image URL')}
                  placeholder='https://example.com/character.png'
                  value={url}
                  disabled={!selectedApiKey || apiKeyValueQuery.isLoading}
                  onChange={(event) => setURL(event.target.value)}
                />
                <Button
                  type='submit'
                  disabled={
                    createMutation.isPending || !selectedApiKey || !url.trim()
                  }
                >
                  {t('Create')}
                </Button>
              </form>
            </CardContent>
          </Card>
          <Card>
            <CardHeader>
              <h2 className='text-base font-medium'>{t('Your assets')}</h2>
            </CardHeader>
            <CardContent className='p-0'>
              <Table>
                <TableHeader>
                  <TableRow>
                    <TableHead>{t('Preview')}</TableHead>
                    <TableHead>{t('Asset ID')}</TableHead>
                    <TableHead>{t('Status')}</TableHead>
                    <TableHead>{t('Created')}</TableHead>
                    <TableHead className='text-right'>{t('Actions')}</TableHead>
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {items.map((asset) => (
                    <TableRow key={asset.id}>
                      <TableCell>
                        <Dialog>
                          <DialogTrigger
                            type='button'
                            aria-label={t('Image Preview')}
                            className='group focus-visible:ring-ring block cursor-zoom-in rounded outline-none focus-visible:ring-2 focus-visible:ring-offset-2'
                          >
                            <img
                              src={asset.url}
                              alt=''
                              className='size-12 rounded object-cover transition-opacity group-hover:opacity-80'
                            />
                          </DialogTrigger>
                          <DialogContent className='w-fit max-w-[calc(100vw-2rem)] p-2 sm:max-w-[calc(100vw-4rem)]'>
                            <DialogTitle className='sr-only'>
                              {t('Image Preview')}
                            </DialogTitle>
                            <img
                              src={asset.url}
                              alt={t('Image Preview')}
                              className='max-h-[calc(100vh-4rem)] max-w-full rounded object-contain'
                            />
                          </DialogContent>
                        </Dialog>
                      </TableCell>
                      <TableCell>
                        <CopyButton
                          value={asset.id}
                          size='sm'
                          tooltip={t('Copy to clipboard')}
                          className='h-auto max-w-full justify-start px-1 py-1 font-mono text-xs font-normal'
                          iconClassName='size-3.5'
                        >
                          {asset.id}
                        </CopyButton>
                      </TableCell>
                      <TableCell>
                        <Badge variant={statusVariant(asset.status)}>
                          {t(asset.status)}
                        </Badge>
                      </TableCell>
                      <TableCell>
                        {new Date(asset.created_at * 1000).toLocaleString()}
                      </TableCell>
                      <TableCell className='text-right'>
                        <div className='flex justify-end gap-1'>
                          <Button
                            variant='ghost'
                            size='icon'
                            aria-label={t('Refresh')}
                            disabled={refreshMutation.isPending}
                            onClick={() =>
                              refreshMutation.mutate({
                                apiKey: selectedApiKey,
                                tokenID: selectedKeyId,
                                asset,
                              })
                            }
                          >
                            <RefreshCw className='size-4' />
                          </Button>
                          <Button
                            variant='ghost'
                            size='icon'
                            aria-label={t('Copy reference')}
                            onClick={() => void copyReference(asset)}
                            disabled={!asset.reference}
                          >
                            <Copy className='size-4' />
                          </Button>
                        </div>
                      </TableCell>
                    </TableRow>
                  ))}
                </TableBody>
              </Table>
              {emptyMessage ? (
                <p
                  className={
                    emptyMessageIsError
                      ? 'text-destructive p-6 text-center text-sm'
                      : 'text-muted-foreground p-6 text-center text-sm'
                  }
                >
                  {emptyMessage}
                </p>
              ) : null}
            </CardContent>
          </Card>
        </div>
      </SectionPageLayout.Content>
    </SectionPageLayout>
  )
}
