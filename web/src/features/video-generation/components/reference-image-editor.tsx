/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as published by
the Free Software Foundation, either version 3 of the License, or
(at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program. If not, see <https://www.gnu.org/licenses/>.

For commercial licensing, please contact support@quantumnous.com
*/
import { useEffect } from 'react'
import { useTranslation } from 'react-i18next'

import { ToggleGroup, ToggleGroupItem } from '@/components/ui/toggle-group'

import { supportsRoleAssets } from '../lib/request'
import type { VideoImageSource } from '../types'
import { AssetPicker } from './asset-picker'
import { ReferenceMediaEditor } from './reference-media-editor'

const ROLE_ASSET_LIMIT = 9

type ReferenceImageEditorProps = {
  model: string
  source: VideoImageSource
  imageUrls: string[]
  assetIds: string[]
  apiKeyId: number | null
  apiKey: string
  imageLimit: number
  onSourceChange: (source: VideoImageSource) => void
  onImageUrlsChange: (urls: string[]) => void
  onAssetIdsChange: (ids: string[]) => void
}

export function ReferenceImageEditor(props: ReferenceImageEditorProps) {
  const { t } = useTranslation()
  const roleAssetsSupported =
    props.model.length === 0 || supportsRoleAssets(props.model)
  const source = props.source
  const onSourceChange = props.onSourceChange

  useEffect(() => {
    if (source === 'asset' && props.model && !roleAssetsSupported) {
      onSourceChange('url')
    }
  }, [source, onSourceChange, props.model, roleAssetsSupported])

  return (
    <div className='space-y-3'>
      <ToggleGroup
        aria-label={t('Image source')}
        value={[props.source]}
        variant='outline'
        className='w-full'
        onValueChange={(values) => {
          const next = values.find((value) => value !== props.source)
          if (next === 'url' || (next === 'asset' && roleAssetsSupported)) {
            props.onSourceChange(next)
          }
        }}
      >
        <ToggleGroupItem value='url' className='flex-1'>
          {t('Public URLs')}
        </ToggleGroupItem>
        <ToggleGroupItem
          value='asset'
          className='flex-1'
          disabled={!roleAssetsSupported}
        >
          {t('Asset library')}
        </ToggleGroupItem>
      </ToggleGroup>

      {!roleAssetsSupported ? (
        <p className='text-muted-foreground text-xs'>
          {t('Role assets are available only for the base Seedance 2.0 model.')}
        </p>
      ) : null}

      {props.source === 'asset' ? (
        <AssetPicker
          apiKeyId={props.apiKeyId}
          apiKey={props.apiKey}
          selectedIds={props.assetIds}
          limit={Math.min(props.imageLimit, ROLE_ASSET_LIMIT)}
          onChange={props.onAssetIdsChange}
        />
      ) : (
        <ReferenceMediaEditor
          kind='images'
          values={props.imageUrls}
          limit={props.imageLimit}
          onChange={props.onImageUrlsChange}
        />
      )}
    </div>
  )
}
