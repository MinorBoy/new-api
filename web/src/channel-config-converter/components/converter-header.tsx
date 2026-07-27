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
import { useTranslation } from 'react-i18next'

export function ConverterHeader() {
  const { t } = useTranslation()

  return (
    <header style={{ borderBottom: '1px solid #d7dee8', paddingBottom: 20 }}>
      <p style={{ color: '#526173', fontSize: 13, margin: '0 0 8px' }}>
        {t('Channel configuration')}
      </p>
      <h1 style={{ fontSize: 30, lineHeight: 1.2, margin: 0 }}>
        {t('Offline workbook converter')}
      </h1>
    </header>
  )
}
