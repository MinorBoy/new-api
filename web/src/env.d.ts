/// <reference types="@rsbuild/core/types" />

declare module '@visactor/react-vchart' {
  export const VChart: React.ComponentType<Record<string, unknown>>
}

// Documentation Markdown files under features/docs/content are inlined as raw
// strings via the rsbuild `asset/source` rule (see rsbuild.config.ts).
declare module '*.md' {
  const content: string
  export default content
}
