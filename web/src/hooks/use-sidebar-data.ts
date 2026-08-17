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
import { useQuery } from '@tanstack/react-query'
import {
  Activity,
  BellRing,
  Box,
  Brush,
  CreditCard,
  ExternalLink,
  FileText,
  FlaskConical,
  Gamepad2,
  HandCoins,
  Key,
  LayoutDashboard,
  ListTodo,
  MessageSquare,
  Puzzle,
  Radio,
  ReceiptText,
  ServerCog,
  Settings,
  ShieldCheck,
  Ticket,
  User,
  Users,
  Wallet,
} from 'lucide-react'
import { useTranslation } from 'react-i18next'

import type { SidebarData } from '@/components/layout/types'
import { getExtensions } from '@/features/extensions/api'
import { useStatus } from '@/hooks/use-status'
import { getCanvasSettingsFromSidebarModules } from '@/lib/canvas-settings'
import {
  getCustomNavIcon,
  getSidebarCustomModuleKey,
  parseCustomNavItems,
} from '@/lib/custom-nav'
import { parseSidebarModulesFromStatus } from '@/lib/nav-modules'
import { ROLE } from '@/lib/roles'
import { useAuthStore } from '@/stores/auth-store'

/** Root navigation groups shown outside contextual drill-in sidebars. */
export function useSidebarData(): SidebarData {
  const { t } = useTranslation()
  const { status } = useStatus()
  const user = useAuthStore((state) => state.auth.user)
  const { data: extensionData } = useQuery({
    queryKey: ['extensions'],
    queryFn: () => getExtensions(),
    enabled: Boolean(user),
    staleTime: 30_000,
  })
  const rawStatus = status as Record<string, unknown> | null
  const sidebarModules = parseSidebarModulesFromStatus(rawStatus)
  const canvasSettings = getCanvasSettingsFromSidebarModules(
    rawStatus?.SidebarModulesAdmin
  )
  const CanvasIcon = getCustomNavIcon(canvasSettings.canvasIcon) ?? Brush
  const customItems = parseCustomNavItems(sidebarModules.customItems)

  const enabledExtensionNavItems =
    extensionData?.modules
      ?.filter((module) => module.enabled)
      .flatMap((module) =>
        (module.ui?.nav ?? []).map((navItem, index) => ({
          module,
          navItem,
          index,
        }))
      )
      .sort((left, right) => {
        const orderDifference =
          (left.navItem.order ?? left.index) -
          (right.navItem.order ?? right.index)
        return orderDifference || left.module.id.localeCompare(right.module.id)
      }) ?? []

  const extensionMenuItems = [
    ...(user && user.role >= ROLE.SUPER_ADMIN
      ? [
          {
            title: t('Module Management'),
            url: '/extensions',
            icon: Settings,
            configUrls: ['/extensions'],
          },
        ]
      : []),
    ...enabledExtensionNavItems.map(({ module, navItem }) => ({
      title: navItem.title,
      url: `/extensions/${module.id}/${navItem.page}`,
      icon: getCustomNavIcon(navItem.icon) ?? Puzzle,
      configUrls:
        module.id === 'channel-quality' && navItem.page === 'index'
          ? ['/channel-observability']
          : [`extension:${module.id}:${navItem.page}`],
    })),
  ]

  const sidebarData: SidebarData = {
    navGroups: [
      {
        id: 'chat',
        title: t('Chat'),
        items: [
          {
            title: t('Playground'),
            url: '/playground',
            icon: FlaskConical,
          },
          {
            title: t('Infinite Canvas'),
            url: '/canvas',
            icon: CanvasIcon,
          },
          {
            title: t('Chat'),
            icon: MessageSquare,
            type: 'chat-presets',
          },
        ],
      },
      {
        id: 'console',
        title: t('General'),
        items: [
          {
            title: t('Overview'),
            url: '/dashboard/overview',
            icon: Activity,
          },
          {
            title: t('Dashboard'),
            url: '/dashboard/models',
            icon: LayoutDashboard,
          },
          { title: t('API Keys'), url: '/keys', icon: Key },
          {
            title: t('Usage Logs'),
            url: '/usage-logs/common',
            icon: FileText,
          },
          {
            title: t('Task Logs'),
            url: '/usage-logs/task',
            activeUrls: ['/usage-logs/drawing'],
            configUrls: ['/usage-logs/drawing', '/usage-logs/task'],
            icon: ListTodo,
          },
          {
            title: t('Game Center'),
            url: '/game-center',
            activeUrls: ['/game-center'],
            icon: Gamepad2,
          },
        ],
      },
      {
        id: 'personal',
        title: t('Personal'),
        items: [
          { title: t('Wallet'), url: '/wallet', icon: Wallet },
          { title: t('Invoice Center'), url: '/invoices', icon: ReceiptText },
          {
            title: t('Affiliate Commission'),
            url: '/affiliate',
            icon: HandCoins,
          },
          { title: t('Profile'), url: '/profile', icon: User },
        ],
      },
      {
        id: 'admin',
        title: t('Admin'),
        items: [
          { title: t('Channels'), url: '/channels', icon: Radio },
          { title: t('Models'), url: '/models/metadata', icon: Box },
          { title: t('Users'), url: '/users', icon: Users },
          {
            title: t('Security Audit'),
            url: '/security-audit',
            icon: ShieldCheck,
            requiredRole: ROLE.SUPER_ADMIN,
          },
          {
            title: t('Redemption Codes'),
            url: '/redemption-codes',
            icon: Ticket,
          },
          {
            title: t('Subscriptions'),
            url: '/subscriptions',
            icon: CreditCard,
          },
          {
            title: t('Billing & Payment'),
            url: '/invoice-management',
            icon: CreditCard,
            requiredRole: ROLE.ADMIN,
          },
          {
            title: t('Notification Center'),
            url: '/notification-center',
            icon: BellRing,
            requiredRole: ROLE.SUPER_ADMIN,
          },
          {
            title: t('Affiliate Commission'),
            url: '/system-settings/billing/affiliate',
            icon: HandCoins,
          },
          {
            title: t('Game Management'),
            url: '/game-management',
            icon: Gamepad2,
          },
          {
            title: t('System Info'),
            url: '/system-info',
            icon: ServerCog,
            requiredRole: ROLE.SUPER_ADMIN,
          },
          {
            title: t('System Settings'),
            url: '/system-settings/site',
            activeUrls: ['/system-settings'],
            icon: Settings,
          },
        ],
      },
    ],
  }

  customItems.forEach((item) => {
    const group = sidebarData.navGroups.find(
      (navGroup) => navGroup.id === item.section
    )
    if (!group) return
    group.items.push({
      title: item.title,
      url: item.url,
      icon: getCustomNavIcon(item.icon) ?? ExternalLink,
      external: item.external,
      configUrls: [getSidebarCustomModuleKey(item.id)],
    })
  })

  const adminGroup = sidebarData.navGroups.find(
    (navGroup) => navGroup.id === 'admin'
  )
  const systemSettingsItem = adminGroup?.items.find(
    (item) => 'url' in item && item.url === '/system-settings/site'
  )
  const isAdminUser = Boolean(user && user.role >= ROLE.ADMIN)

  if (adminGroup && isAdminUser && extensionMenuItems.length > 0) {
    const insertionIndex = systemSettingsItem
      ? adminGroup.items.indexOf(systemSettingsItem)
      : adminGroup.items.length
    adminGroup.items.splice(insertionIndex, 0, {
      title: t('Extension Modules'),
      icon: Puzzle,
      items: extensionMenuItems,
    })
  } else if (extensionMenuItems.length > 0) {
    sidebarData.navGroups.push({
      id: 'extensions',
      title: t('Modules'),
      items: [
        {
          title: t('Extension Modules'),
          icon: Puzzle,
          items: extensionMenuItems,
        },
      ],
    })
  }

  if (adminGroup && systemSettingsItem) {
    adminGroup.items = [
      ...adminGroup.items.filter((item) => item !== systemSettingsItem),
      systemSettingsItem,
    ]
  }

  return sidebarData
}
