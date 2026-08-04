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
*/
import { Check, Copy } from 'lucide-react'
import { useTranslation } from 'react-i18next'

import { Dialog } from '@/components/dialog'
import { Button } from '@/components/ui/button'
import { ScrollArea } from '@/components/ui/scroll-area'
import { useCopyToClipboard } from '@/hooks/use-copy-to-clipboard'

interface TaskAuditDataDialogProps {
  title: string
  formattedData: string
  open: boolean
  onOpenChange: (open: boolean) => void
}

export function TaskAuditDataDialog(props: TaskAuditDataDialogProps) {
  const { t } = useTranslation()
  const { copiedText, copyToClipboard } = useCopyToClipboard({ notify: false })

  return (
    <Dialog
      open={props.open}
      onOpenChange={props.onOpenChange}
      title={t(props.title)}
      description={t('View the full data captured for this task.')}
      contentClassName='sm:max-w-3xl'
      contentHeight='auto'
      bodyClassName='space-y-4'
    >
      <ScrollArea className='max-h-[65vh] pr-4'>
        <div className='bg-muted/40 relative rounded-md border p-3'>
          <Button
            variant='ghost'
            size='sm'
            className='absolute top-2 right-2 h-8 w-8 p-0'
            onClick={() => copyToClipboard(props.formattedData)}
            title={t('Copy to clipboard')}
            aria-label={t('Copy to clipboard')}
          >
            {copiedText === props.formattedData ? (
              <Check className='size-4 text-green-600' />
            ) : (
              <Copy className='size-4' />
            )}
          </Button>
          <pre className='overflow-wrap-anywhere min-w-0 pr-10 font-mono text-xs leading-relaxed break-all whitespace-pre-wrap'>
            {props.formattedData || '-'}
          </pre>
        </div>
      </ScrollArea>
    </Dialog>
  )
}
