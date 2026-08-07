export type GroupRoutingRequirements = {
  require_real_person?: boolean
}

export function updateGroupRoutingRequirements(
  source: string,
  groupName: string,
  requireRealPerson: boolean
): string {
  const normalizedGroupName = groupName.trim()
  if (!normalizedGroupName) {
    throw new Error('Group name is required')
  }

  let value: unknown
  try {
    value = JSON.parse(source.trim() || '{}')
  } catch {
    throw new Error('Group routing requirements must be valid JSON')
  }
  if (typeof value !== 'object' || value === null || Array.isArray(value)) {
    throw new Error('Group routing requirements must be a JSON object')
  }
  const parsed = value as Record<string, GroupRoutingRequirements>

  parsed[normalizedGroupName] = {
    ...parsed[normalizedGroupName],
    require_real_person: requireRealPerson,
  }

  return JSON.stringify(
    Object.fromEntries(
      Object.entries(parsed).sort(([left], [right]) =>
        left.localeCompare(right)
      )
    ),
    null,
    2
  )
}
