export type CodeTab = {
  /** Source language label, e.g. `curl`, `python`, `node`. */
  lang: string
  /** CodeMirror/shiki language for highlighting. */
  highlight: string
  /** Display label shown on the tab. */
  label: string
  code: string
}

/**
 * Map a fenced-code language to its display label and highlight language.
 * Anything not listed here falls through to itself.
 */
export function codeTabMeta(lang: string): {
  label: string
  highlight: string
} {
  const key = lang.toLowerCase()
  const known: Record<string, { label: string; highlight: string }> = {
    curl: { label: 'cURL', highlight: 'bash' },
    bash: { label: 'Bash', highlight: 'bash' },
    shell: { label: 'Shell', highlight: 'bash' },
    sh: { label: 'Shell', highlight: 'bash' },
    python: { label: 'Python', highlight: 'python' },
    py: { label: 'Python', highlight: 'python' },
    node: { label: 'Node.js', highlight: 'javascript' },
    javascript: { label: 'JavaScript', highlight: 'javascript' },
    js: { label: 'JavaScript', highlight: 'javascript' },
    typescript: { label: 'TypeScript', highlight: 'typescript' },
    ts: { label: 'TypeScript', highlight: 'typescript' },
    go: { label: 'Go', highlight: 'go' },
    json: { label: 'JSON', highlight: 'json' },
    http: { label: 'HTTP', highlight: 'http' },
  }
  return known[key] ?? { label: lang || 'Text', highlight: lang || 'plaintext' }
}
