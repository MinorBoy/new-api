import { lexer, type Tokens } from 'marked'

export type DocHeading = {
  id: string
  text: string
  level: number
}

/**
 * Slugify a heading into a URL anchor. Matches the GitHub-flavored algorithm
 * closely enough for the on-this-page navigation: lowercase, trim, drop
 * non-word characters except hyphens, spaces → hyphens.
 */
export function slugifyHeading(text: string): string {
  return text
    .toLowerCase()
    .trim()
    .replaceAll(/[^\p{L}\p{N}\s-]/gu, '')
    .replaceAll(/\s+/g, '-')
    .replaceAll(/-+/g, '-')
    .replaceAll(/^-|-$/g, '')
}

/**
 * Extract H2/H3 headings from markdown for the on-this-page panel.
 * Pure function over the token stream — no DOM needed.
 */
export function extractDocHeadings(markdown: string): DocHeading[] {
  const headings: DocHeading[] = []
  for (const token of lexer(markdown)) {
    if (token.type !== 'heading') {
      continue
    }
    const heading = token as Tokens.Heading
    if (heading.depth !== 2 && heading.depth !== 3) {
      continue
    }
    headings.push({
      id: slugifyHeading(heading.text),
      text: heading.text,
      level: heading.depth,
    })
  }
  return headings
}
