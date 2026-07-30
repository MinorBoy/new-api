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
import assert from 'node:assert/strict'
import test, { after, beforeEach } from 'node:test'

// @ts-expect-error Bun supplies mock.module at test runtime, but the frontend
// typecheck intentionally only includes Node's test declarations.
import { mock } from 'bun:test'
import { Window } from 'happy-dom'
import { act } from 'react'
import type { Container, Root } from 'react-dom/client'

const browserWindow = new Window({ url: 'http://localhost/' })
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

mock.module('@/components/config-drawer', () => ({
  ConfigDrawer: () => <div data-header-control='theme' />,
}))
mock.module('@/components/language-switcher', () => ({
  LanguageSwitcher: () => <div data-header-control='language' />,
}))
mock.module('@/components/notification-popover', () => ({
  NotificationPopover: () => <div data-header-control='notifications' />,
}))
mock.module('@/components/profile-dropdown', () => ({
  ProfileDropdown: () => <div data-header-control='profile' />,
}))
mock.module('@/components/search', () => ({
  Search: () => <div data-header-control='search' />,
}))
mock.module('@/components/wallet-balance-button', () => ({
  WalletBalanceLink: () => <div data-header-control='wallet-balance' />,
}))
mock.module('@/hooks/use-notifications', () => ({
  useNotifications: () => ({
    popoverOpen: false,
    setPopoverOpen: () => {},
    unreadCount: 0,
    activeTab: 'notice',
    setActiveTab: () => {},
    notice: '',
    announcements: [],
    loading: false,
  }),
}))
mock.module('@/hooks/use-top-nav-links', () => ({
  useTopNavLinks: () => [],
}))
mock.module('../header', () => ({
  Header: ({ children }: { children: React.ReactNode }) => <header>{children}</header>,
}))
mock.module('../system-brand', () => ({
  SystemBrand: () => <div />,
}))
mock.module('../top-nav', () => ({
  TopNav: () => <div data-header-control='top-nav' />,
}))

const { createRoot } = await import('react-dom/client')
const { AppHeader } = await import('../app-header')

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
})

async function mountHeader() {
  const container = browserWindow.document.createElement('div')
  browserWindow.document.body.append(container)
  const root = createRoot(container as unknown as Container)

  await act(async () => {
    root.render(<AppHeader showTopNav={false} />)
  })

  return { container: container as unknown as HTMLElement, root }
}

async function unmountHeader(mounted: { root: Root; container: HTMLElement }) {
  await act(async () => mounted.root.unmount())
  mounted.container.remove()
}

test('places the wallet balance between theme settings and the account menu', async () => {
  const mounted = await mountHeader()
  const controls = [
    ...mounted.container.querySelectorAll('[data-header-control]'),
  ].map((control) => (control as HTMLElement).dataset.headerControl)

  assert.deepEqual(controls, [
    'search',
    'notifications',
    'language',
    'theme',
    'wallet-balance',
    'profile',
  ])

  await unmountHeader(mounted)
})
