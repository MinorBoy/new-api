import { useState, type ReactNode } from 'react'

import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'

import { DocsCodeBlock } from '../components/docs-code-block'
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
    return (
      <DocsCodeBlock
        code={tab.code}
        language={tab.highlight}
        label={tab.label}
      />
    )
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
          <DocsCodeBlock
            code={tab.code}
            language={tab.highlight}
            label={tab.label}
          />
        </TabsContent>
      ))}
    </Tabs>
  )
}

export type { CodeTab } from './code-tabs-meta'
