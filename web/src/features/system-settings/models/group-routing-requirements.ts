export type GroupRoutingProfileStatus = 'draft' | 'active'
export type GroupRealPersonMode = 'any' | 'required' | 'forbidden'
export type GroupCostMode =
  | 'per_request'
  | 'per_duration'
  | 'per_token'
  | 'free'

export type GroupRoutingRequirements = {
  [key: string]: unknown
  require_real_person?: boolean
  status?: GroupRoutingProfileStatus
  routing_source?: 'default'
  real_person_mode?: GroupRealPersonMode
  allowed_cost_modes?: GroupCostMode[]
  excluded_target_keys?: string[]
}

export type GroupRoutingProfiles = Record<string, GroupRoutingRequirements>

const DYNAMIC_PROFILE_FIELDS = [
  'status',
  'routing_source',
  'real_person_mode',
  'allowed_cost_modes',
  'excluded_target_keys',
] as const

export function parseGroupRoutingProfiles(
  source: string
): GroupRoutingProfiles {
  let value: unknown
  try {
    value = JSON.parse(source.trim() || '{}')
  } catch {
    throw new Error('Group routing requirements must be valid JSON')
  }
  if (!isRecord(value)) {
    throw new Error('Group routing requirements must be a JSON object')
  }

  const profiles: GroupRoutingProfiles = {}
  for (const [groupName, profile] of Object.entries(value)) {
    if (!isRecord(profile)) {
      throw new Error(
        `Group routing requirements for ${groupName} must be an object`
      )
    }
    profiles[groupName] = { ...profile }
  }
  return profiles
}

export function serializeGroupRoutingProfiles(
  profiles: GroupRoutingProfiles
): string {
  const normalizedEntries = Object.entries(profiles)
    .map(
      ([groupName, profile]) => [groupName, normalizeProfile(profile)] as const
    )
    .sort(([left], [right]) => compareStrings(left, right))
  return JSON.stringify(Object.fromEntries(normalizedEntries), null, 2)
}

export function updateGroupRoutingProfile(
  source: string,
  groupName: string,
  profile: GroupRoutingRequirements
): string {
  const normalizedGroupName = requireGroupName(groupName)
  const profiles = parseGroupRoutingProfiles(source)
  profiles[normalizedGroupName] = {
    ...profiles[normalizedGroupName],
    ...profile,
  }
  return serializeGroupRoutingProfiles(profiles)
}

export function removeDynamicGroupRoutingProfile(
  source: string,
  groupName: string
): string {
  const normalizedGroupName = requireGroupName(groupName)
  const profiles = parseGroupRoutingProfiles(source)
  const profile = profiles[normalizedGroupName]
  if (!profile) {
    return serializeGroupRoutingProfiles(profiles)
  }

  const nextProfile = { ...profile }
  for (const field of DYNAMIC_PROFILE_FIELDS) {
    delete nextProfile[field]
  }
  if (Object.keys(nextProfile).length === 0) {
    delete profiles[normalizedGroupName]
  } else {
    profiles[normalizedGroupName] = nextProfile
  }
  return serializeGroupRoutingProfiles(profiles)
}

export function effectiveRealPersonMode(
  profile: GroupRoutingRequirements | undefined
): GroupRealPersonMode {
  if (profile?.real_person_mode) {
    return profile.real_person_mode
  }
  return profile?.require_real_person === true ? 'required' : 'any'
}

export function isDynamicGroupRoutingProfile(
  profile: GroupRoutingRequirements | undefined
): boolean {
  return profile?.routing_source === 'default'
}

export function toggleGroupRoutingTargetExclusion(
  source: string,
  groupName: string,
  targetKey: string,
  excluded: boolean
): string {
  const normalizedGroupName = requireGroupName(groupName)
  const normalizedTargetKey = targetKey.trim()
  if (!normalizedTargetKey) {
    throw new Error('Target key is required')
  }
  const profiles = parseGroupRoutingProfiles(source)
  const profile = profiles[normalizedGroupName]
  if (!profile) {
    throw new Error(`Group routing profile ${normalizedGroupName} is required`)
  }

  const targetKeys = new Set(profile.excluded_target_keys ?? [])
  if (excluded) {
    targetKeys.add(normalizedTargetKey)
  } else {
    targetKeys.delete(normalizedTargetKey)
  }
  profiles[normalizedGroupName] = {
    ...profile,
    excluded_target_keys: [...targetKeys],
  }
  return serializeGroupRoutingProfiles(profiles)
}

export function updateGroupRoutingRequirements(
  source: string,
  groupName: string,
  requireRealPerson: boolean
): string {
  const normalizedGroupName = requireGroupName(groupName)
  const profiles = parseGroupRoutingProfiles(source)
  profiles[normalizedGroupName] = {
    ...profiles[normalizedGroupName],
    require_real_person: requireRealPerson,
  }
  return serializeGroupRoutingProfiles(profiles)
}

function normalizeProfile(
  profile: GroupRoutingRequirements
): GroupRoutingRequirements {
  const normalized = { ...profile }
  if (normalized.real_person_mode === 'any') {
    delete normalized.real_person_mode
  }
  normalized.allowed_cost_modes = normalizeStringArray(
    normalized.allowed_cost_modes
  ) as GroupCostMode[]
  normalized.excluded_target_keys = normalizeStringArray(
    normalized.excluded_target_keys
  )
  if (normalized.allowed_cost_modes.length === 0) {
    delete normalized.allowed_cost_modes
  }
  if (normalized.excluded_target_keys.length === 0) {
    delete normalized.excluded_target_keys
  }
  return normalized
}

function normalizeStringArray(values: string[] | undefined): string[] {
  return [...new Set(values ?? [])].sort(compareStrings)
}

function compareStrings(left: string, right: string): number {
  if (left === right) {
    return 0
  }
  return left < right ? -1 : 1
}

function requireGroupName(groupName: string): string {
  const normalizedGroupName = groupName.trim()
  if (!normalizedGroupName) {
    throw new Error('Group name is required')
  }
  return normalizedGroupName
}

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === 'object' && value !== null && !Array.isArray(value)
}
