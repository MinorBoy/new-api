import { useState, type ReactNode } from 'react'

import {
  CodeBlock,
  CodeBlockCopyButton,
} from '@/components/ai-elements/code-block'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'

import type { CodeTab } from './code-tabs-meta'

/**
 * Render one or more fenced code blocks. A single block renders as a plain
 * `<CodeBlock>`; two or more render as language tabs with a shared copy button,
 * mirroring the cURL / Python / Node switcher used on reference docs sites.
 */
export function CodeTabs({ tabs }: { tabs: CodeTab[] }): ReactNode {
  const [value, setValue] = useState(tabs.at(0)?.lang ?? '')

  if (tabs.length === 0) {
    return null
  }

  if (tabs.length === 1) {
    const tab = tabs[0]
    return (
      <CodeBlock code={tab.code} language={tab.highlight} showToolbar>
        <CodeBlockCopyButton />
      </CodeBlock>
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
          <CodeBlock code={tab.code} language={tab.highlight} showToolbar>
            <CodeBlockCopyButton />
          </CodeBlock>
        </TabsContent>
      ))}
    </Tabs>
  )
}

export type { CodeTab } from './code-tabs-meta'
