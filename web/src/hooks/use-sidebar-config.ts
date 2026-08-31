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
import { useMemo } from 'react'

import type { NavGroup, NavItem } from '@/components/layout/types'
import { useStatus } from '@/hooks/use-status'
import {
  getSidebarCustomModuleKey,
  parseCustomNavItems,
} from '@/lib/custom-nav'
import { parseHeaderNavBoolean } from '@/lib/nav-modules'
import { useAuthStore, type UserPermissions } from '@/stores/auth-store'

export type SidebarSectionConfig = {
  enabled: boolean
  [key: string]: boolean | string
}

export type SidebarModulesAdminConfig = Record<string, SidebarSectionConfig>
export type SidebarModulesUserConfig = SidebarModulesAdminConfig | null
type SidebarModulesPermissionConfig = UserPermissions['sidebar_modules']

const DEFAULT_SIDEBAR_MODULES: SidebarModulesAdminConfig = {
  chat: {
    enabled: true,
    playground: true,
    canvas: true,
    chat: true,
  },
  console: {
    enabled: true,
    detail: true,
    token: true,
    log: true,
    midjourney: true,
    task: true,
    game: true,
  },
  personal: {
    enabled: true,
    topup: true,
    invoice: true,
    affiliate: true,
    benefits: true,
    personal: true,
  },
  admin: {
    enabled: true,
    channel: true,
    channel_observability: true,
    models: true,
    redemption: true,
    user: true,
    affiliate_admin: true,
    setting: true,
    subscription: true,
    invoice_admin: true,
    notification_center: true,
    extension_admin: true,
    security_audit: true,
    game: true,
  },
}

const URL_TO_CONFIG_MAP: Record<string, { section: string; module: string }> = {
  '/playground': { section: 'chat', module: 'playground' },
  '/canvas': { section: 'chat', module: 'canvas' },
  '/dashboard': { section: 'console', module: 'detail' },
  '/dashboard/overview': { section: 'console', module: 'detail' },
  '/dashboard/models': { section: 'console', module: 'detail' },
  '/dashboard/users': { section: 'console', module: 'detail' },
  '/keys': { section: 'console', module: 'token' },
  '/usage-logs': { section: 'console', module: 'log' },
  '/usage-logs/common': { section: 'console', module: 'log' },
  '/usage-logs/drawing': { section: 'console', module: 'midjourney' },
  '/usage-logs/task': { section: 'console', module: 'task' },
  '/game-center': { section: 'console', module: 'game' },
  '/wallet': { section: 'personal', module: 'topup' },
  '/invoices': { section: 'personal', module: 'invoice' },
  '/affiliate': { section: 'personal', module: 'affiliate' },
  '/profile': { section: 'personal', module: 'personal' },
  '/channels': { section: 'admin', module: 'channel' },
  '/channel-observability': {
    section: 'admin',
    module: 'channel_observability',
  },
  '/models': { section: 'admin', module: 'models' },
  '/users': { section: 'admin', module: 'user' },
  '/redemption-codes': { section: 'admin', module: 'redemption' },
  '/benefits': { section: 'personal', module: 'benefits' },
  '/subscriptions': { section: 'admin', module: 'subscription' },
  '/invoice-management': { section: 'admin', module: 'invoice_admin' },
  '/notification-center': {
    section: 'admin',
    module: 'notification_center',
  },
  '/extensions': { section: 'admin', module: 'extension_admin' },
  '/security-audit': { section: 'admin', module: 'security_audit' },
  '/system-settings/billing/affiliate': {
    section: 'admin',
    module: 'affiliate_admin',
  },
  '/game-management': { section: 'admin', module: 'game' },
  '/system-settings': { section: 'admin', module: 'setting' },
}

const URL_CONFIG_PREFIXES = Object.entries(URL_TO_CONFIG_MAP).sort(
  ([left], [right]) => right.length - left.length
)
const CUSTOM_MODULE_PREFIX = 'custom:'

function parseRecord(value: unknown): Record<string, unknown> | null {
  if (!value || String(value).trim() === '') return null
  if (typeof value === 'object' && !Array.isArray(value)) {
    return value as Record<string, unknown>
  }
  try {
    const parsed = JSON.parse(String(value)) as unknown
    return parsed && typeof parsed === 'object' && !Array.isArray(parsed)
      ? (parsed as Record<string, unknown>)
      : null
  } catch {
    return null
  }
}

export function parseSidebarConfig(value: unknown): SidebarModulesAdminConfig {
  const config = Object.fromEntries(
    Object.entries(DEFAULT_SIDEBAR_MODULES).map(([section, modules]) => [
      section,
      { ...modules },
    ])
  ) as SidebarModulesAdminConfig
  const parsed = parseRecord(value)
  if (!parsed) return config

  Object.entries(parsed).forEach(([sectionKey, sectionValue]) => {
    if (
      sectionKey === 'customItems' ||
      !sectionValue ||
      typeof sectionValue !== 'object' ||
      Array.isArray(sectionValue)
    ) {
      return
    }

    const record = sectionValue as Record<string, unknown>
    const section: SidebarSectionConfig = {
      ...(config[sectionKey] ?? { enabled: true }),
      enabled: parseHeaderNavBoolean(
        record.enabled,
        config[sectionKey]?.enabled ?? true
      ),
    }
    Object.entries(record).forEach(([moduleKey, moduleValue]) => {
      if (moduleKey === 'enabled') return
      if (typeof moduleValue === 'boolean') {
        section[moduleKey] = moduleValue
      }
    })
    config[sectionKey] = section
  })

  const customItems = parseCustomNavItems(parsed.customItems)
  if (customItems.length > 0) {
    const custom = config.custom ?? { enabled: true }
    customItems.forEach((item) => {
      const key = getSidebarCustomModuleKey(item.id)
      if (custom[key] === undefined) custom[key] = true
    })
    config.custom = custom
  }

  return config
}

export function parseUserSidebarConfig(
  value: unknown
): SidebarModulesUserConfig {
  const parsed = parseRecord(value)
  if (!parsed) return null

  const config: SidebarModulesAdminConfig = {}
  Object.entries(parsed).forEach(([sectionKey, sectionValue]) => {
    if (
      !sectionValue ||
      typeof sectionValue !== 'object' ||
      Array.isArray(sectionValue)
    ) {
      return
    }

    const section: SidebarSectionConfig = { enabled: true }
    Object.entries(sectionValue as Record<string, unknown>).forEach(
      ([moduleKey, moduleValue]) => {
        if (typeof moduleValue === 'boolean') section[moduleKey] = moduleValue
      }
    )
    config[sectionKey] = section
  })
  return config
}

function isConfiguredModuleEnabled(
  section: string,
  module: string,
  adminConfig: SidebarModulesAdminConfig,
  userConfig: SidebarModulesUserConfig,
  permissionConfig: SidebarModulesPermissionConfig
): boolean {
  const adminSection = adminConfig[section]
  if (
    !adminSection ||
    adminSection.enabled === false ||
    adminSection[module] !== true
  ) {
    return false
  }

  const permissionSection = permissionConfig?.[section]
  if (permissionSection === false) return false
  if (permissionSection && typeof permissionSection === 'object') {
    const permissions = permissionSection as Record<string, unknown>
    if (permissions.enabled === false || permissions[module] === false) {
      return false
    }
  }

  const userSection = userConfig?.[section]
  if (!userSection) return true
  return userSection.enabled !== false && userSection[module] !== false
}

export function isModuleEnabled(
  url: string,
  adminConfig: SidebarModulesAdminConfig,
  userConfig: SidebarModulesUserConfig,
  permissionConfig?: SidebarModulesPermissionConfig
): boolean {
  if (url.startsWith(CUSTOM_MODULE_PREFIX)) {
    const adminCustom = adminConfig.custom
    if (
      adminCustom &&
      (adminCustom.enabled === false || adminCustom[url] === false)
    ) {
      return false
    }
    const permissionCustom = permissionConfig?.custom
    if (permissionCustom === false) return false
    if (permissionCustom && typeof permissionCustom === 'object') {
      const permissions = permissionCustom as Record<string, unknown>
      if (permissions.enabled === false || permissions[url] === false) {
        return false
      }
    }
    const userCustom = userConfig?.custom
    return Boolean(
      !userCustom || (userCustom.enabled !== false && userCustom[url] !== false)
    )
  }

  const normalizedUrl =
    url.endsWith('/') && url !== '/' ? url.slice(0, -1) : url
  const mapping =
    URL_TO_CONFIG_MAP[normalizedUrl] ??
    URL_CONFIG_PREFIXES.find(([prefix]) =>
      normalizedUrl.startsWith(`${prefix}/`)
    )?.[1]
  if (!mapping) return true

  return isConfiguredModuleEnabled(
    mapping.section,
    mapping.module,
    adminConfig,
    userConfig,
    permissionConfig
  )
}

function isNavItemVisible(
  item: NavItem,
  adminConfig: SidebarModulesAdminConfig,
  userConfig: SidebarModulesUserConfig,
  permissionConfig: SidebarModulesPermissionConfig
): boolean {
  if ('type' in item && item.type === 'chat-presets') {
    return isConfiguredModuleEnabled(
      'chat',
      'chat',
      adminConfig,
      userConfig,
      permissionConfig
    )
  }

  if ('url' in item && item.url) {
    const configUrls = item.configUrls ?? [item.url]
    return configUrls.some((url) =>
      isModuleEnabled(String(url), adminConfig, userConfig, permissionConfig)
    )
  }

  if ('items' in item && item.items) return item.items.length > 0
  return true
}

function filterNavItems(
  items: NavItem[],
  adminConfig: SidebarModulesAdminConfig,
  userConfig: SidebarModulesUserConfig,
  permissionConfig: SidebarModulesPermissionConfig
): NavItem[] {
  return items
    .map((item) => {
      if (!('items' in item) || !item.items) return item
      return {
        ...item,
        items: item.items.filter((subItem) => {
          const configUrls = subItem.configUrls ?? [subItem.url]
          return configUrls.some((url) =>
            isModuleEnabled(
              String(url),
              adminConfig,
              userConfig,
              permissionConfig
            )
          )
        }),
      }
    })
    .filter((item) =>
      isNavItemVisible(item, adminConfig, userConfig, permissionConfig)
    )
}

export function filterSidebarNavGroups(
  navGroups: NavGroup[],
  adminConfig: SidebarModulesAdminConfig,
  userConfig: SidebarModulesUserConfig,
  permissionConfig?: SidebarModulesPermissionConfig
): NavGroup[] {
  return navGroups
    .map((group) => ({
      ...group,
      items: filterNavItems(
        group.items,
        adminConfig,
        userConfig,
        permissionConfig
      ),
    }))
    .filter((group) => group.items.length > 0)
}

export function useSidebarConfig(navGroups: NavGroup[]): NavGroup[] {
  const { status } = useStatus()
  const { auth } = useAuthStore()

  const adminConfig = useMemo(
    () => parseSidebarConfig(status?.SidebarModulesAdmin),
    [status?.SidebarModulesAdmin]
  )
  const userConfig = useMemo(
    () =>
      auth?.user?.permissions?.sidebar_settings === false
        ? null
        : parseUserSidebarConfig(auth?.user?.sidebar_modules),
    [auth?.user?.permissions?.sidebar_settings, auth?.user?.sidebar_modules]
  )
  const permissionConfig = auth?.user?.permissions?.sidebar_modules

  return useMemo(
    () =>
      filterSidebarNavGroups(
        navGroups,
        adminConfig,
        userConfig,
        permissionConfig
      ),
    [navGroups, adminConfig, userConfig, permissionConfig]
  )
}

export function useIsSidebarModuleVisible(url: string): boolean {
  const { status } = useStatus()
  const { auth } = useAuthStore()
  const adminConfig = parseSidebarConfig(status?.SidebarModulesAdmin)
  const userConfig =
    auth?.user?.permissions?.sidebar_settings === false
      ? null
      : parseUserSidebarConfig(auth?.user?.sidebar_modules)

  return isModuleEnabled(
    url,
    adminConfig,
    userConfig,
    auth?.user?.permissions?.sidebar_modules
  )
}
