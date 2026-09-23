// @ts-expect-error Bun supplies mock.module at test runtime, but the frontend
// typecheck intentionally only includes Node's test declarations.
import { mock } from 'bun:test'
import assert from 'node:assert/strict'
import test, { after, beforeEach } from 'node:test'

import { Window } from 'happy-dom'
import { act, createElement, type ReactNode } from 'react'
import type { Container, Root } from 'react-dom/client'

const browserWindow = new Window({ url: 'http://localhost/dashboard' })
const browserGlobals = {
  window: browserWindow,
  document: browserWindow.document,
  navigator: browserWindow.navigator,
  Element: browserWindow.Element,
  HTMLElement: browserWindow.HTMLElement,
  Node: browserWindow.Node,
  Event: browserWindow.Event,
  MutationObserver: browserWindow.MutationObserver,
  ResizeObserver: browserWindow.ResizeObserver,
  IntersectionObserver: browserWindow.IntersectionObserver,
  getComputedStyle: browserWindow.getComputedStyle.bind(browserWindow),
  requestAnimationFrame:
    browserWindow.requestAnimationFrame.bind(browserWindow),
  cancelAnimationFrame: browserWindow.cancelAnimationFrame.bind(browserWindow),
  IS_REACT_ACT_ENVIRONMENT: true,
}
const previousBrowserGlobals = Object.fromEntries(
  Object.keys(browserGlobals).map((key) => [
    key,
    Object.getOwnPropertyDescriptor(globalThis, key),
  ])
)
Object.assign(globalThis as Record<string, unknown>, browserGlobals)

type MockProps = {
  children?: ReactNode
  [key: string]: unknown
}

function passthrough({ children, ...props }: MockProps) {
  return createElement('div', props, children)
}

let studioClicks = 0
const navigateCalls: string[] = []
const searchOpenChanges: boolean[] = []

mock.module('@/components/ui/command', () => ({
  Command: passthrough,
  CommandDialog: ({ children, open }: MockProps & { open?: boolean }) =>
    open
      ? createElement('div', { 'data-command-dialog': 'true' }, children)
      : null,
  CommandEmpty: passthrough,
  CommandGroup: passthrough,
  CommandInput: (props: MockProps) => createElement('input', props),
  CommandItem: ({ children, onSelect, value }: MockProps) =>
    createElement(
      'button',
      {
        type: 'button',
        'data-command-value': value,
        onClick: () =>
          (onSelect as ((value: string) => void) | undefined)?.(
            String(value ?? '')
          ),
      },
      children
    ),
  CommandList: passthrough,
  CommandSeparator: () => null,
}))
mock.module('@/components/ui/scroll-area', () => ({
  ScrollArea: passthrough,
}))
mock.module('@/components/layout/lib/sidebar-view-registry', () => ({
  getNavGroupsForPath: () => null,
}))
mock.module('@/context/search-provider', () => ({
  useSearch: () => ({
    open: true,
    setOpen: (open: boolean) => searchOpenChanges.push(open),
  }),
}))
mock.module('@/context/theme-provider', () => ({
  useTheme: () => ({
    setTheme: () => undefined,
  }),
}))
mock.module('@/hooks/use-sidebar-data', () => ({
  useSidebarData: () => ({
    navGroups: [
      {
        id: 'chat',
        title: 'Chat',
        items: [
          {
            title: 'Creative Studio',
            url: '/video-generation',
            onClick: () => {
              studioClicks += 1
            },
          },
          {
            title: 'Video generation',
            url: '/video-generation',
          },
        ],
      },
    ],
  }),
}))
mock.module('@tanstack/react-router', () => ({
  useLocation: () => ({ pathname: '/dashboard' }),
  useNavigate: () => (options: { to: string }) => {
    navigateCalls.push(options.to)
  },
}))
mock.module('react-i18next', () => ({
  useTranslation: () => ({ t: (key: string) => key }),
}))

const { createRoot } = await import('react-dom/client')
const { CommandMenu } = await import('../command-menu')

after(() => {
  for (const key of Object.keys(browserGlobals)) {
    const previousDescriptor = previousBrowserGlobals[key]
    if (previousDescriptor === undefined) {
      delete (globalThis as Record<string, unknown>)[key]
      continue
    }
    Object.defineProperty(globalThis, key, previousDescriptor)
  }
  browserWindow.close()
})

beforeEach(() => {
  browserWindow.document.body.replaceChildren()
  studioClicks = 0
  navigateCalls.length = 0
  searchOpenChanges.length = 0
})

async function mountCommandMenu() {
  const container = browserWindow.document.createElement('div')
  browserWindow.document.body.append(container)
  const root = createRoot(container as unknown as Container)

  await act(async () => root.render(<CommandMenu />))

  return { container: container as unknown as HTMLElement, root }
}

async function unmountCommandMenu(mounted: {
  root: Root
  container: HTMLElement
}) {
  await act(async () => mounted.root.unmount())
  mounted.container.remove()
}

test('runs a navigation item onClick without navigating to its fallback URL', async () => {
  const mounted = await mountCommandMenu()

  const studioItem = [...mounted.container.querySelectorAll('button')].find(
    (button) => button.textContent?.includes('Creative Studio')
  )
  assert.ok(studioItem)

  await act(async () => studioItem.click())

  assert.equal(studioClicks, 1)
  assert.deepEqual(navigateCalls, [])
  assert.deepEqual(searchOpenChanges, [false])

  await unmountCommandMenu(mounted)
})
