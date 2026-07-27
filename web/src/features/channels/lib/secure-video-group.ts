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
export const SECURE_CHANNEL_TYPE = 207

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

export function filterSecureAddModeOptions<T extends { value: string }>(
  type: number,
  options: readonly T[]
): readonly T[] {
  if (type !== SECURE_CHANNEL_TYPE) return options
  return options.filter((option) => option.value !== 'multi_to_single')
}

export function shouldLockSecureVideoIdentity(
  isEditing: boolean,
  originalType?: number
): boolean {
  return isEditing && originalType === SECURE_CHANNEL_TYPE
}
