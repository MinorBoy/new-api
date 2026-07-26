export const SECURE_CHANNEL_TYPE = 66

export const SECURE_VIDEO_GROUP_OPTIONS = [
  { value: 'discount', label: 'Discount Video' },
  { value: 'overseas', label: 'Overseas Video' },
  { value: 'enterprise', label: 'Enterprise Video' },
] as const

export type SecureVideoGroup =
  (typeof SECURE_VIDEO_GROUP_OPTIONS)[number]['value']

export function shouldShowSecureVideoGroup(type: number): boolean {
  return type === SECURE_CHANNEL_TYPE
}
