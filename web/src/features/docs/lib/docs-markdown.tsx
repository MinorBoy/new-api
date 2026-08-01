import { lexer, type Token, type Tokens } from 'marked'
import { Fragment, type ReactNode, useMemo } from 'react'

import { renderMarkdown } from '@/components/ui/markdown'
import { cn } from '@/lib/utils'

import { CodeTabs, type CodeTab } from './code-tabs'
import { codeTabMeta } from './code-tabs-meta'
import { slugifyHeading } from './headings'

type RenderedSegment =
  | { kind: 'prose'; markdown: string; key: string }
  | { kind: 'code'; tabs: CodeTab[]; key: string }

function isCodeToken(token: Token): token is Tokens.Code {
  return token.type === 'code'
}

/**
 * Split markdown into a render plan: prose segments (rendered through the
 * shared markdown pipeline, preserving GFM tables, KaTeX, diagrams) interleaved
 * with code segments (one or more adjacent fenced blocks turned into tabs).
 *
 * "Adjacent" fenced blocks may be separated by blank-line `space` tokens, which
 * are consumed into the code run so that ```` ```curl ```` + ```` ```python ````
 * collapse into a single tabbed block — the cURL/Python/Node switcher used by
 * reference docs sites. Two adjacent blocks of the *same* language instead end
 * the run and start a new one, since they are separate snippets.
 */
function planSegments(markdown: string): RenderedSegment[] {
  const tokens = lexer(markdown)
  const segments: RenderedSegment[] = []
  let proseBuffer = ''
  let codeBuffer: CodeTab[] = []
  let segmentIndex = 0

  const flushProse = () => {
    if (!proseBuffer.trim()) {
      proseBuffer = ''
      return
    }
    segments.push({
      kind: 'prose',
      markdown: proseBuffer,
      key: `prose-${segmentIndex++}`,
    })
    proseBuffer = ''
  }

  const flushCode = () => {
    if (codeBuffer.length === 0) {
      return
    }
    segments.push({
      kind: 'code',
      tabs: codeBuffer,
      key: `code-${segmentIndex++}`,
    })
    codeBuffer = []
  }

  for (const token of tokens) {
    if (isCodeToken(token)) {
      const lang = token.lang ?? ''
      // A fenced block with the same language as the last buffered one ends the
      // current run and starts a new one — two `bash` blocks are separate
      // snippets, while `curl` + `python` are alternate forms of one example.
      const lastLang = codeBuffer.at(-1)?.lang
      if (codeBuffer.length > 0 && lang === lastLang) {
        flushCode()
      } else {
        flushProse()
      }
      const meta = codeTabMeta(lang)
      codeBuffer.push({
        lang,
        highlight: meta.highlight,
        label: meta.label,
        code: token.text,
      })
      continue
    }

    if (token.type === 'space' && codeBuffer.length > 0) {
      // Blank line between adjacent fenced blocks belongs to the code run.
      continue
    }

    // Any other token ends an open code run before accumulating as prose.
    flushCode()
    proseBuffer += token.raw ?? ''
  }

  flushProse()
  flushCode()

  return segments
}

/**
 * Render a markdown document with enhanced code blocks: fenced code becomes a
 * React `<CodeBlock>` (CodeMirror highlight + copy button), and adjacent blocks
 * in different languages collapse into language tabs. Prose keeps the existing
 * GFM/KaTeX/diagram styling.
 *
 * Headings get anchor ids post-sanitization so the on-this-page links resolve.
 */
export function DocsMarkdown({
  source,
  className,
}: {
  source: string
  className?: string
}): ReactNode {
  const segments = useMemo(() => planSegments(source), [source])

  return (
    <div
      className={cn(
        'prose prose-sm dark:prose-invert max-w-none',
        // Reuse the same prose styling as the shared Markdown component so
        // headings, lists, tables, blockquotes, and inline code match.
        '[&_h1]:mt-6 [&_h1]:mb-3 [&_h1]:text-2xl [&_h1]:font-semibold',
        '[&_h2]:mt-5 [&_h2]:mb-3 [&_h2]:scroll-mt-24 [&_h2]:text-xl [&_h2]:font-semibold',
        '[&_h3]:mt-4 [&_h3]:mb-2 [&_h3]:scroll-mt-24 [&_h3]:text-lg [&_h3]:font-semibold',
        '[&_h4]:mt-4 [&_h4]:mb-2 [&_h4]:font-semibold',
        '[&_p]:my-2 [&_p]:leading-relaxed [&_strong]:font-semibold [&_em]:italic',
        '[&_a]:text-primary [&_a]:underline hover:[&_a]:text-primary/80',
        '[&_ol]:my-2 [&_ul]:my-2 [&_ol]:list-decimal [&_ul]:list-disc [&_ol]:pl-5 [&_ul]:pl-5 [&_li]:my-1 [&_li]:pl-1',
        '[&_blockquote]:my-3 [&_blockquote]:border-l-2 [&_blockquote]:border-primary [&_blockquote]:bg-muted/50 [&_blockquote]:py-1 [&_blockquote]:pl-4',
        '[&_code]:rounded [&_code]:bg-muted [&_code]:px-1 [&_code]:py-0.5 [&_code]:font-mono',
        '[&_table]:my-4 [&_table]:block [&_table]:w-full [&_table]:overflow-x-auto',
        '[&_thead]:bg-muted [&_th]:border [&_td]:border [&_th]:px-3 [&_td]:px-3 [&_th]:py-2 [&_td]:py-2 [&_th]:text-left',
        '[&_hr]:my-6 [&_img]:my-4 [&_img]:max-w-full [&_img]:rounded-lg',
        '[&>*:first-child]:mt-0 [&>*:last-child]:mb-0',
        '[overflow-wrap:anywhere]',
        className
      )}
    >
      {segments.map((segment) =>
        segment.kind === 'prose' ? (
          <Fragment key={segment.key}>
            <ProseBlock markdown={segment.markdown} />
          </Fragment>
        ) : (
          <div className='not-prose my-4' key={segment.key}>
            <CodeTabs tabs={segment.tabs} />
          </div>
        )
      )}
    </div>
  )
}

/**
 * Render one prose segment to sanitized HTML and inject heading anchor ids.
 *
 * The shared `renderMarkdown` runs DOMPurify, which strips `id` attributes by
 * default. For the on-this-page panel to work we re-add ids to H2/H3 via a
 * post-process on the parsed DOM, mirroring the anchor convention in
 * `extractDocHeadings`.
 */
function ProseBlock({ markdown }: { markdown: string }): ReactNode {
  const html = useMemo(() => {
    const rendered = renderMarkdown(markdown)
    return injectHeadingIds(rendered)
  }, [markdown])

  return <div dangerouslySetInnerHTML={{ __html: html }} />
}

/**
 * Add `id` anchors to H2/H3 headings in rendered HTML so the on-this-page links
 * resolve. Safe to run after sanitization because it only touches heading tags
 * and writes slug ids derived from their text content.
 */
function injectHeadingIds(html: string): string {
  if (typeof window === 'undefined' || !html.includes('<h')) {
    return html
  }
  const template = document.createElement('template')
  template.innerHTML = html
  template.content.querySelectorAll('h2, h3').forEach((heading) => {
    heading.id = slugifyHeading(heading.textContent ?? '')
  })
  return template.innerHTML
}
