import { AudioLines, Clapperboard, Image, Plus, X } from 'lucide-react'
import { useRef } from 'react'
import { useTranslation } from 'react-i18next'

import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'

import { VIDEO_MEDIA_LIMITS } from '../lib/request'
import type { VideoMedia } from '../types'

type MediaKind = keyof VideoMedia

type ReferenceMediaEditorProps = {
  kind: MediaKind
  values: string[]
  onChange: (values: string[]) => void
}

const MEDIA_COPY: Record<MediaKind, { label: string; hint: string }> = {
  images: {
    label: 'Reference images',
    hint: 'JPG, PNG, or any public image URL',
  },
  videos: {
    label: 'Reference videos',
    hint: 'MP4 or another public video URL',
  },
  audios: { label: 'Reference audio', hint: 'MP3 or another public audio URL' },
}

const MEDIA_ICONS = {
  images: Image,
  videos: Clapperboard,
  audios: AudioLines,
} as const

export function ReferenceMediaEditor(props: ReferenceMediaEditorProps) {
  const { t } = useTranslation()
  const Icon = MEDIA_ICONS[props.kind]
  const copy = MEDIA_COPY[props.kind]
  const limit = VIDEO_MEDIA_LIMITS[props.kind]
  const rowIds = useRef<string[]>([])
  while (rowIds.current.length < props.values.length) {
    rowIds.current.push(crypto.randomUUID())
  }
  if (rowIds.current.length > props.values.length) {
    rowIds.current.length = props.values.length
  }

  function updateValue(index: number, value: string) {
    props.onChange(
      props.values.map((current, currentIndex) =>
        currentIndex === index ? value : current
      )
    )
  }

  function addValue() {
    if (props.values.length >= limit) return
    rowIds.current.push(crypto.randomUUID())
    props.onChange([...props.values, ''])
  }

  function removeValue(index: number) {
    rowIds.current.splice(index, 1)
    props.onChange(
      props.values.filter((_, currentIndex) => currentIndex !== index)
    )
  }

  return (
    <section className='bg-muted/20 rounded-lg border p-3'>
      <div className='mb-3 flex items-center justify-between gap-2'>
        <div className='flex min-w-0 items-center gap-2'>
          <Icon className='text-primary size-4 shrink-0' aria-hidden='true' />
          <div className='min-w-0'>
            <h3 className='truncate text-sm font-medium'>{t(copy.label)}</h3>
            <p className='text-muted-foreground truncate text-xs'>
              {t(copy.hint)}
            </p>
          </div>
        </div>
        <span className='text-muted-foreground shrink-0 text-xs tabular-nums'>
          {props.values.length} / {limit}
        </span>
      </div>

      <div className='space-y-2'>
        {props.values.map((value, index) => (
          <div className='flex items-center gap-2' key={rowIds.current[index]}>
            <Input
              aria-label={`${t(copy.label)} ${index + 1}`}
              inputMode='url'
              placeholder='https://'
              value={value}
              onChange={(event) => updateValue(index, event.target.value)}
            />
            <Button
              aria-label={t('Remove reference URL')}
              className='shrink-0'
              size='icon'
              type='button'
              variant='ghost'
              onClick={() => removeValue(index)}
            >
              <X aria-hidden='true' />
            </Button>
          </div>
        ))}
      </div>

      <Button
        className='mt-3 w-full'
        disabled={props.values.length >= limit}
        size='sm'
        type='button'
        variant='outline'
        onClick={addValue}
      >
        <Plus aria-hidden='true' />
        {t('Add URL')}
      </Button>
    </section>
  )
}
