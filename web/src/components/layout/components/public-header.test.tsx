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
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import {
  createElement,
  forwardRef,
  type ComponentProps,
  type MouseEvent,
} from 'react'
import { describe, expect, test, vi } from 'vitest'

import type { TopNavLink } from '../types'
import { PublicHeader } from './public-header'

vi.mock('@tanstack/react-router', () => {
  type MockLinkProps = ComponentProps<'a'> & {
    to: string
    disabled?: boolean
  }

  const Link = forwardRef<HTMLAnchorElement, MockLinkProps>(function MockLink(
    { to, disabled, onClick, ...props },
    ref
  ) {
    return createElement('a', {
      ...props,
      ref,
      href: to,
      'aria-disabled': disabled || undefined,
      onClick: (event: MouseEvent<HTMLAnchorElement>) => {
        event.preventDefault()
        if (!disabled) onClick?.(event)
      },
    })
  })

  return {
    Link,
    useNavigate: () => vi.fn(),
    useRouterState: () => ({ location: { pathname: '/' } }),
  }
})

vi.mock('@/components/dialog', () => ({ Dialog: () => null }))
vi.mock('@/hooks/use-notifications', () => ({
  useNotifications: () => ({}),
}))
vi.mock('@/hooks/use-system-config', () => ({
  useSystemConfig: () => ({
    systemName: 'New API',
    logo: '/logo.png',
    loading: false,
    logoLoaded: true,
  }),
}))
vi.mock('@/hooks/use-top-nav-links', () => ({
  useTopNavLinks: () => [],
}))
vi.mock('@/stores/auth-store', () => ({
  useAuthStore: () => ({ auth: { user: null } }),
}))

const navLinks: TopNavLink[] = [
  { title: 'Home', href: '/' },
  { title: 'About', href: '/about' },
]

function renderHeader() {
  render(
    <PublicHeader
      navLinks={navLinks}
      showThemeSwitch={false}
      showLanguageSwitcher={false}
      showNotifications={false}
    />
  )
  const toggle = screen.getByRole('button', {
    name: 'Toggle navigation menu',
  })
  const navigation = document.querySelector<HTMLElement>(
    '#public-mobile-navigation'
  )
  if (!navigation) throw new Error('Expected mobile navigation')
  return { navigation, toggle }
}

describe('PublicHeader mobile navigation', () => {
  test('removes the closed menu from focus and exposes its disclosure state', async () => {
    const user = userEvent.setup()
    const { navigation, toggle } = renderHeader()
    const links = [...navigation.querySelectorAll('a')]

    expect(toggle).toHaveAttribute('aria-expanded', 'false')
    expect(toggle).toHaveAttribute('aria-controls', 'public-mobile-navigation')
    expect(navigation).toHaveAttribute('inert')
    expect(navigation).toHaveAttribute('aria-hidden', 'true')
    expect(links).not.toHaveLength(0)
    for (const link of links) expect(link).toHaveAttribute('tabindex', '-1')

    await user.click(toggle)

    expect(toggle).toHaveAttribute('aria-expanded', 'true')
    expect(navigation).not.toHaveAttribute('inert')
    expect(navigation).toHaveAttribute('aria-hidden', 'false')
    for (const link of links) expect(link).not.toHaveAttribute('tabindex')
    await waitFor(() => expect(links[0]).toHaveFocus())

    await user.keyboard('{Escape}')

    expect(toggle).toHaveAttribute('aria-expanded', 'false')
    expect(toggle).toHaveFocus()
    expect(navigation).toHaveAttribute('inert')
    for (const link of links) expect(link).toHaveAttribute('tabindex', '-1')
  })

  test('keeps mobile links usable and closes after navigation', async () => {
    const user = userEvent.setup()
    const { navigation, toggle } = renderHeader()
    const aboutLink =
      navigation.querySelector<HTMLAnchorElement>('a[href="/about"]')
    if (!aboutLink) throw new Error('Expected About link')

    await user.click(toggle)
    await waitFor(() => expect(aboutLink).not.toHaveAttribute('tabindex'))
    await user.click(aboutLink)

    expect(toggle).toHaveAttribute('aria-expanded', 'false')
    expect(navigation).toHaveAttribute('inert')
    expect(aboutLink).toHaveAttribute('tabindex', '-1')
  })
})
