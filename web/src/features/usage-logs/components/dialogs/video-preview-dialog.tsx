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
import { Loader2, RotateCcw, Video } from 'lucide-react'
import { useTranslation } from 'react-i18next'

import { Dialog } from '@/components/dialog'
import { Button } from '@/components/ui/button'
import { IconBadge } from '@/components/ui/icon-badge'

interface VideoPreviewDialogProps {
  open: boolean
  onOpenChange: (open: boolean) => void
  mediaURL: string
  loading: boolean
  failed: boolean
  onRetry: () => void
}

export function VideoPreviewDialog(props: VideoPreviewDialogProps) {
  const { t } = useTranslation()

  let content = (
    <div className='text-muted-foreground flex min-h-48 items-center justify-center gap-2 text-sm'>
      <Loader2 className='size-4 animate-spin' aria-hidden='true' />
      {t('Loading...')}
    </div>
  )
  if (props.failed) {
    content = (
      <div className='flex min-h-48 flex-col items-center justify-center gap-3'>
        <p className='text-destructive text-sm'>{t('Failed to load')}</p>
        <Button variant='outline' size='sm' onClick={props.onRetry}>
          <RotateCcw className='size-4' aria-hidden='true' />
          {t('Retry')}
        </Button>
      </div>
    )
  } else if (!props.loading && props.mediaURL) {
    content = (
      <video
        src={props.mediaURL}
        controls
        preload='metadata'
        className='max-h-[70vh] w-full bg-black object-contain'
      />
    )
  }

  return (
    <Dialog
      open={props.open}
      onOpenChange={props.onOpenChange}
      title={
        <>
          <IconBadge tone='chart-4' size='sm'>
            <Video />
          </IconBadge>
          {t('Preview')}
        </>
      }
      contentClassName='sm:max-w-3xl'
      titleClassName='flex items-center gap-2'
      contentHeight='auto'
      bodyClassName='overflow-hidden p-0'
    >
      {content}
    </Dialog>
  )
}
