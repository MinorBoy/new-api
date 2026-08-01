// @ts-expect-error Bun supplies mock.module at test runtime; the frontend
// typecheck only includes Node's test declarations.
import { mock } from 'bun:test'
import assert from 'node:assert/strict'
import { describe, test } from 'node:test'

import { createElement } from 'react'
import { renderToStaticMarkup } from 'react-dom/server'

import { codeTabMeta } from '../lib/code-tabs-meta'
import { DocsMarkdown } from '../lib/docs-markdown'
import { extractDocHeadings, slugifyHeading } from '../lib/headings'

// Stub the shared prose pipeline so these tests isolate DocsMarkdown's own
// contract (splitting code vs prose, mounting CodeBlock vs Tabs) without
// depending on DOMPurify/marked DOM behavior under a server test harness.
// The marker element lets prose segments be located in the rendered output.
mock.module('@/components/ui/markdown', () => ({
  renderMarkdown: (markdown: string) =>
    `<div class="prose-stub">${markdown}</div>`,
}))

// CodeBlock mounts a live CodeMirror editor, which is heavyweight and DOM-bound.
// For these contract tests we only care that code segments turn into a CodeBlock
// vs. language Tabs, so we stub CodeBlock to a custom element whose markup we
// can assert against.
mock.module('@/components/ai-elements/code-block', () => ({
  CodeBlock: ({ code, language }: { code: string; language: string }) =>
    createElement('code-block', { 'data-lang': language }, code),
  CodeBlockCopyButton: () => createElement('copy-button', null),
}))

function render(source: string): string {
  return renderToStaticMarkup(<DocsMarkdown source={source} />)
}

describe('DocsMarkdown code blocks', () => {
  test('renders a single fenced block as a standalone CodeBlock', () => {
    const html = render('Intro.\n\n```python\nprint(1)\n```\n')
    assert.match(
      html,
      /<code-block data-lang="python">print\(1\)<\/code-block>/
    )
    // Prose around it is preserved.
    assert.match(html, /Intro\./)
  })

  test('collapses adjacent multi-language blocks into tabs', () => {
    const html = render(
      '```curl\ncurl https://x\n```\n\n```python\nimport x\n```\n'
    )
    // Both languages produce tab triggers (cURL + Python).
    assert.match(html, />cURL</)
    assert.match(html, />Python</)
    // The active (first) tab's code renders; Base UI lazily mounts inactive
    // panels, so we assert the first tab's content rather than both.
    assert.match(
      html,
      /<code-block data-lang="bash">curl https:\/\/x<\/code-block>/
    )
  })

  test('does not collapse two blocks of the same language into one tab pair', () => {
    // Two identical-language blocks separated by a blank line are still
    // adjacent tokens, so both render as standalone CodeBlocks (single-block
    // path) rather than a tabbed pair.
    const html = render('```bash\na\n```\n\n```bash\nb\n```\n')
    assert.match(html, /<code-block data-lang="bash">a<\/code-block>/)
    assert.match(html, /<code-block data-lang="bash">b<\/code-block>/)
    // No tablist should be present.
    assert.doesNotMatch(html, /role="tablist"/)
  })
})

describe('DocsMarkdown prose preservation', () => {
  test('passes prose segments through the shared renderMarkdown pipeline', () => {
    const html = render('- item one\n- **bold** item\n')
    // The stub wraps raw markdown verbatim, so list/emphasis source survives.
    assert.match(html, /prose-stub/)
    assert.match(html, /item one/)
    assert.match(html, /\*\*bold\*\* item/)
  })

  test('renders inline code distinctly from fenced blocks', () => {
    const html = render(
      'Use `npm install`, then:\n\n```bash\nnpm install\n```\n'
    )
    // Inline code stays in the prose stub; the fenced block becomes CodeBlock.
    assert.match(html, /`npm install`/)
    assert.match(html, /<code-block data-lang="bash">npm install<\/code-block>/)
  })

  test('separates prose segments from code segments in document order', () => {
    const html = render('Before code.\n\n```bash\nrun me\n```\n\nAfter code.\n')
    const before = html.indexOf('Before code.')
    const code = html.indexOf('run me')
    const after = html.indexOf('After code.')
    assert.ok(before !== -1 && code !== -1 && after !== -1)
    assert.ok(before < code, 'prose before code should precede the code block')
    assert.ok(code < after, 'code should precede prose after it')
  })
})

describe('extractDocHeadings', () => {
  test('collects only H2/H3 with slug ids', () => {
    const headings = extractDocHeadings(
      '# H1 skipped\n\n## Getting Started\n\n### Details Here\n\n#### H4 skipped\n'
    )
    assert.equal(headings.length, 2)
    assert.deepEqual(
      headings.map((h) => h.id),
      ['getting-started', 'details-here']
    )
    assert.deepEqual(
      headings.map((h) => h.level),
      [2, 3]
    )
  })

  test('handles non-ASCII headings', () => {
    const headings = extractDocHeadings('## 快速接入\n')
    assert.equal(headings[0].id, '快速接入')
  })
})

describe('slugifyHeading', () => {
  test('lowercases, hyphenates, strips punctuation', () => {
    assert.equal(slugifyHeading('Quick Start!'), 'quick-start')
    assert.equal(slugifyHeading('API Reference (v1)'), 'api-reference-v1')
    assert.equal(slugifyHeading('  multiple   spaces  '), 'multiple-spaces')
  })
})

describe('codeTabMeta', () => {
  test('maps common aliases to highlight languages', () => {
    assert.equal(codeTabMeta('curl').highlight, 'bash')
    assert.equal(codeTabMeta('node').highlight, 'javascript')
    assert.equal(codeTabMeta('ts').highlight, 'typescript')
    assert.equal(codeTabMeta('py').label, 'Python')
  })

  test('falls back to the raw language when unknown', () => {
    assert.equal(codeTabMeta('dockerfile').highlight, 'dockerfile')
    assert.equal(codeTabMeta('dockerfile').label, 'dockerfile')
  })

  test('falls back to plaintext for empty language', () => {
    assert.equal(codeTabMeta('').highlight, 'plaintext')
  })
})
