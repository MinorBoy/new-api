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
'use client'

import { codeToHtml } from 'shiki'
import { useEffect, useState, type ReactNode } from 'react'

import { CopyButton } from '@/components/copy-button'
import { cn } from '@/lib/utils'

/**
 * Highlighted, read-only code block for the docs platform, using Shiki (VS Code
 * grammar quality) with a synchronized light/dark theme via CSS variables.
 *
 * Unlike the shared editor-grade `<CodeBlock>` (CodeMirror, no language grammars
 * wired up for bash/python/js), this component is purpose-built for static
 * documentation display: better highlighting, lighter weight, and a theme that
 * tracks the site light/dark switch automatically.
 */
export function DocsCodeBlock({
  code,
  language,
  label,
  className,
}: {
  code: string
  /** Shiki grammar name, e.g. bash / python / javascript / json. */
  language: string
  /** Optional toolbar label; falls back to the language. */
  label?: string
  className?: string
}): ReactNode {
  const [html, setHtml] = useState<string>('')

  useEffect(() => {
    let cancelled = false
    // Shiki ships its own bundled grammars; unknown languages fall back to text.
    codeToHtml(code, {
      lang: language,
      themes: { light: 'github-light', dark: 'github-dark' },
      defaultColor: false,
    })
      .then((rendered) => {
        if (!cancelled) setHtml(rendered)
      })
      .catch(() => {
        if (!cancelled) setHtml('')
      })
    return () => {
      cancelled = true
    }
  }, [code, language])

  return (
    <div
      className={cn(
        'group relative my-4 overflow-hidden rounded-lg border bg-background',
        className
      )}
    >
      <div className='bg-muted/40 text-muted-foreground flex items-center justify-between border-b px-3 py-1.5'>
        <span className='font-mono text-[11px] font-medium tracking-wide uppercase'>
          {label ?? language}
        </span>
        <CopyButton
          value={code}
          variant='ghost'
          size='icon'
          className='size-6 opacity-60 group-hover:opacity-100'
        />
      </div>
      {/* Render Shiki HTML when ready; show plain <pre> as a non-blocking fallback
          so the layout never collapses while the highlighter resolves. */}
      {html ? (
        <div
          className='docs-shiki overflow-x-auto text-[13px] leading-relaxed'
          dangerouslySetInnerHTML={{ __html: html }}
        />
      ) : (
        <pre className='overflow-x-auto p-3 text-[13px] leading-relaxed'>
          <code>{code}</code>
        </pre>
      )}
    </div>
  )
}
