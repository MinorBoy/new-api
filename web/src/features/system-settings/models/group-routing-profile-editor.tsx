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
import { AlertTriangle, ListFilter } from 'lucide-react'
import { useMemo } from 'react'
import { useTranslation } from 'react-i18next'

import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import { Checkbox } from '@/components/ui/checkbox'
import {
  Field,
  FieldDescription,
  FieldGroup,
  FieldLabel,
  FieldLegend,
  FieldSet,
  FieldTitle,
} from '@/components/ui/field'
import { Switch } from '@/components/ui/switch'
import { ToggleGroup, ToggleGroupItem } from '@/components/ui/toggle-group'

import type { GroupRoutingProfileSummary } from './group-routing-profile-api'
import {
  effectiveRealPersonMode,
  isDynamicGroupRoutingProfile,
  parseGroupRoutingProfiles,
  removeDynamicGroupRoutingProfile,
  serializeGroupRoutingProfiles,
  updateGroupRoutingProfile,
  type GroupCostMode,
  type GroupRealPersonMode,
  type GroupRoutingProfileStatus,
} from './group-routing-requirements'

type GroupRoutingProfileEditorProps = {
  groupName: string
  groupRoutingRequirements: string
  summary?: GroupRoutingProfileSummary
  disabled?: boolean
  onChange: (value: string) => void
  onViewTargets: () => void
}

const costModes: GroupCostMode[] = [
  'per_request',
  'per_duration',
  'per_token',
  'free',
]

export function GroupRoutingProfileEditor(
  props: GroupRoutingProfileEditorProps
) {
  const { t } = useTranslation()
  const profiles = useMemo(() => {
    try {
      return parseGroupRoutingProfiles(props.groupRoutingRequirements)
    } catch {
      return {}
    }
  }, [props.groupRoutingRequirements])
  const profile = profiles[props.groupName]
  const dynamic = isDynamicGroupRoutingProfile(profile)
  const realPersonMode = effectiveRealPersonMode(profile)
  const allowedCostModes = profile?.allowed_cost_modes ?? []
  const status = profile?.status ?? 'draft'
  const hasNoCompatibleTargets = props.summary?.matched_targets === 0
  const canAdaptFromDefault =
    props.groupName !== 'default' && props.groupName !== 'auto'
  const adaptDisabled = props.disabled || (!dynamic && !canAdaptFromDefault)

  const updateProfile = (
    changes: Partial<{
      status: GroupRoutingProfileStatus
      real_person_mode: GroupRealPersonMode
      allowed_cost_modes: GroupCostMode[]
    }>
  ) => {
    props.onChange(
      updateGroupRoutingProfile(
        props.groupRoutingRequirements,
        props.groupName,
        {
          ...changes,
          routing_source: 'default',
        }
      )
    )
  }

  const setAdaptFromDefault = (checked: boolean) => {
    if (checked) {
      const nextProfiles = parseGroupRoutingProfiles(
        props.groupRoutingRequirements
      )
      const currentProfile = nextProfiles[props.groupName]
      const initialRealPersonMode =
        currentProfile?.require_real_person === true ? 'required' : 'any'
      nextProfiles[props.groupName] = {
        ...currentProfile,
        status: 'draft',
        routing_source: 'default',
        real_person_mode: initialRealPersonMode,
        allowed_cost_modes: [],
      }
      props.onChange(JSON.stringify(nextProfiles, null, 2))
      return
    }
    props.onChange(
      removeDynamicGroupRoutingProfile(
        props.groupRoutingRequirements,
        props.groupName
      )
    )
  }

  return (
    <FieldGroup>
      <Field
        orientation='horizontal'
        data-disabled={adaptDisabled || undefined}
      >
        <div>
          <button
            type='button'
            className='cursor-pointer text-left text-sm font-medium disabled:cursor-not-allowed disabled:opacity-50'
            disabled={adaptDisabled}
            onClick={() => setAdaptFromDefault(!dynamic)}
          >
            {t('Adapt from default')}
          </button>
          <FieldDescription>
            {canAdaptFromDefault
              ? t(
                  'Build this group routing profile from the default capability catalog.'
                )
              : t('The default and auto groups cannot adapt from default.')}
          </FieldDescription>
        </div>
        <Switch
          checked={dynamic}
          disabled={adaptDisabled}
          onCheckedChange={setAdaptFromDefault}
          aria-label={t('Adapt from default')}
        />
      </Field>

      {dynamic ? (
        <>
          <Field>
            <FieldTitle id='group-routing-real-person-mode'>
              {t('Real-person capability')}
            </FieldTitle>
            <ToggleGroup
              value={[realPersonMode]}
              onValueChange={(selection) => {
                const nextMode = selection[0] as GroupRealPersonMode | undefined
                if (!nextMode) return
                const nextProfiles = parseGroupRoutingProfiles(
                  props.groupRoutingRequirements
                )
                const nextProfile = {
                  ...nextProfiles[props.groupName],
                  routing_source: 'default' as const,
                  real_person_mode: nextMode,
                }
                delete nextProfile.require_real_person
                nextProfiles[props.groupName] = nextProfile
                props.onChange(serializeGroupRoutingProfiles(nextProfiles))
              }}
              disabled={props.disabled}
              aria-labelledby='group-routing-real-person-mode'
              className='flex-wrap'
            >
              <ToggleGroupItem value='any'>{t('Any')}</ToggleGroupItem>
              <ToggleGroupItem value='required'>
                {t('Must support real person')}
              </ToggleGroupItem>
              <ToggleGroupItem value='forbidden'>
                {t('Must not support real person')}
              </ToggleGroupItem>
            </ToggleGroup>
          </Field>

          <FieldSet disabled={props.disabled}>
            <FieldLegend variant='label'>{t('Allowed cost modes')}</FieldLegend>
            <FieldDescription>
              {t('Leave every cost mode unchecked to allow all cost modes.')}
            </FieldDescription>
            <FieldGroup
              data-slot='checkbox-group'
              className='gap-3 sm:grid sm:grid-cols-2'
            >
              {costModes.map((costMode) => {
                const label = {
                  per_request: t('Per request'),
                  per_duration: t('Per duration'),
                  per_token: t('Per token'),
                  free: t('Free'),
                }[costMode]
                return (
                  <Field key={costMode} orientation='horizontal'>
                    <Checkbox
                      id={`group-routing-cost-${costMode}`}
                      checked={allowedCostModes.includes(costMode)}
                      disabled={props.disabled}
                      onCheckedChange={(checked) => {
                        const nextModes = checked
                          ? [...allowedCostModes, costMode]
                          : allowedCostModes.filter((mode) => mode !== costMode)
                        updateProfile({ allowed_cost_modes: nextModes })
                      }}
                      aria-label={label}
                    />
                    <FieldLabel
                      htmlFor={`group-routing-cost-${costMode}`}
                      className='font-normal'
                    >
                      {label}
                    </FieldLabel>
                  </Field>
                )
              })}
            </FieldGroup>
          </FieldSet>

          <Field>
            <FieldTitle id='group-routing-profile-status'>
              {t('Profile status')}
            </FieldTitle>
            <ToggleGroup
              value={[status]}
              onValueChange={(selection) => {
                const nextStatus = selection[0] as
                  | GroupRoutingProfileStatus
                  | undefined
                if (nextStatus) updateProfile({ status: nextStatus })
              }}
              disabled={props.disabled}
              aria-labelledby='group-routing-profile-status'
            >
              <ToggleGroupItem value='draft'>{t('Draft')}</ToggleGroupItem>
              <ToggleGroupItem value='active' disabled={hasNoCompatibleTargets}>
                {t('Active')}
              </ToggleGroupItem>
            </ToggleGroup>
          </Field>

          {hasNoCompatibleTargets ? (
            <Alert>
              <AlertTriangle aria-hidden='true' />
              <AlertTitle>{t('No compatible targets')}</AlertTitle>
              <AlertDescription>
                {t(
                  'At least one compatible target is required before this profile can be activated.'
                )}
              </AlertDescription>
            </Alert>
          ) : null}

          <Button
            type='button'
            variant='outline'
            onClick={props.onViewTargets}
            disabled={props.disabled}
            className='w-fit'
          >
            <ListFilter data-icon='inline-start' aria-hidden='true' />
            {t('View targets')}
          </Button>
        </>
      ) : null}
    </FieldGroup>
  )
}
