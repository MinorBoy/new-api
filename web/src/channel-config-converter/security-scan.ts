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
export type SecretFinding = {
  code: 'SECURITY_SECRET_FIELD' | 'SECURITY_SECRET_VALUE'
  path: string
}

const SECRET_FIELD =
  /(?:^|[_-])(api[_-]?key|authorization|cookie|secret|password|access[_-]?token|refresh[_-]?token)(?:$|[_-])/i
const SECRET_VALUE = /(?:\bbearer\s+|\bbasic\s+|\bsk-[a-z0-9_-]{12,})/i

export function scanForSecrets(value: unknown, path = '$'): SecretFinding[] {
  if (Array.isArray(value)) {
    return value.flatMap((item, index) =>
      scanForSecrets(item, `${path}[${index}]`)
    )
  }
  if (typeof value === 'object' && value !== null) {
    return Object.entries(value).flatMap(([key, nested]) => {
      const nestedPath = `${path}.${key}`
      const findings: SecretFinding[] = []
      if (SECRET_FIELD.test(key)) {
        findings.push({ code: 'SECURITY_SECRET_FIELD', path: nestedPath })
      }
      return [...findings, ...scanForSecrets(nested, nestedPath)]
    })
  }
  if (typeof value === 'string' && SECRET_VALUE.test(value)) {
    return [{ code: 'SECURITY_SECRET_VALUE', path }]
  }
  return []
}
