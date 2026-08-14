import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { Copy, Images, RefreshCw } from 'lucide-react'
import { useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

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
import { Input } from '@/components/ui/input'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'

import { createAsset, listAssets, refreshAsset } from './api'
import type { Asset } from './types'

const queryKey = ['role-assets']

async function loadAssets() {
  const response = await listAssets()
  const processing = response.data.items.filter(
    (asset) => asset.status === 'processing' || asset.status === 'pending'
  )
  if (processing.length === 0) return response
  await Promise.allSettled(processing.map(refreshAsset))
  return listAssets()
}

export function Assets() {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const [url, setURL] = useState('')
  const assetsQuery = useQuery({
    queryKey,
    queryFn: loadAssets,
    refetchInterval: (query) =>
      query.state.data?.data.items.some(
        (asset) => asset.status === 'processing' || asset.status === 'pending'
      )
        ? 2000
        : false,
  })
  const createMutation = useMutation({
    mutationFn: createAsset,
    onSuccess: () => {
      setURL('')
      void queryClient.invalidateQueries({ queryKey })
      toast.success(t('Asset created'))
    },
    onError: () => toast.error(t('Failed to create asset')),
  })
  const refreshMutation = useMutation({
    mutationFn: refreshAsset,
    onSuccess: () => void queryClient.invalidateQueries({ queryKey }),
  })
  const items = assetsQuery.data?.data.items ?? []

  const statusVariant = (status: Asset['status']) => {
    if (status === 'active') return 'default' as const
    if (status === 'failed') return 'destructive' as const
    return 'outline' as const
  }

  useEffect(() => {
    if (assetsQuery.error) toast.error(t('Failed to load assets'))
  }, [assetsQuery.error, t])

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
              <h2 className='text-base font-medium'>
                {t('Create image asset')}
              </h2>
            </CardHeader>
            <CardContent>
              <form
                className='flex flex-col gap-2 sm:flex-row'
                onSubmit={(event) => {
                  event.preventDefault()
                  if (url.trim()) createMutation.mutate(url.trim())
                }}
              >
                <Input
                  aria-label={t('Public image URL')}
                  placeholder='https://example.com/character.png'
                  value={url}
                  onChange={(event) => setURL(event.target.value)}
                />
                <Button
                  type='submit'
                  disabled={createMutation.isPending || !url.trim()}
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
                      <TableCell className='font-mono text-xs'>
                        {asset.id}
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
                            onClick={() => refreshMutation.mutate(asset)}
                            disabled={refreshMutation.isPending}
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
              {items.length === 0 && !assetsQuery.isLoading ? (
                <p className='text-muted-foreground p-6 text-center text-sm'>
                  {t('No assets yet')}
                </p>
              ) : null}
            </CardContent>
          </Card>
        </div>
      </SectionPageLayout.Content>
    </SectionPageLayout>
  )
}
