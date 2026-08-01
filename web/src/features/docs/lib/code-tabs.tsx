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
import { useState, type ReactNode } from 'react'

import { DocsCodeBlock } from '../components/docs-code-block'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'

import type { CodeTab } from './code-tabs-meta'

/**
 * Render one or more fenced code blocks. A single block renders as a plain
 * highlighted block; two or more render as language tabs, mirroring the
 * cURL / Python / Node switcher used on reference docs sites.
 */
export function CodeTabs({ tabs }: { tabs: CodeTab[] }): ReactNode {
  const [value, setValue] = useState(tabs.at(0)?.lang ?? '')

  if (tabs.length === 0) {
    return null
  }

  if (tabs.length === 1) {
    const tab = tabs[0]
    return <DocsCodeBlock code={tab.code} language={tab.highlight} label={tab.label} />
  }

  return (
    <Tabs value={value} onValueChange={setValue}>
      <TabsList variant='line'>
        {tabs.map((tab) => (
          <TabsTrigger key={tab.lang} value={tab.lang}>
            {tab.label}
          </TabsTrigger>
        ))}
      </TabsList>
      {tabs.map((tab) => (
        <TabsContent key={tab.lang} value={tab.lang}>
          <DocsCodeBlock code={tab.code} language={tab.highlight} label={tab.label} />
        </TabsContent>
      ))}
    </Tabs>
  )
}

export type { CodeTab } from './code-tabs-meta'
