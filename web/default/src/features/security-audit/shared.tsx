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
import type { TFunction } from 'i18next'
import { Badge } from '@/components/ui/badge'
import type { SecurityAuditMode } from './types'

export interface SensitiveActionOptions {
  title?: string
  description?: string
}

export type SensitiveActionRunner = (
  action: () => Promise<unknown>,
  options?: SensitiveActionOptions
) => Promise<unknown | null>

export function formatAuditTime(timestamp: number, fallback = '-'): string {
  if (!timestamp) return fallback
  return new Intl.DateTimeFormat(undefined, {
    year: 'numeric',
    month: '2-digit',
    day: '2-digit',
    hour: '2-digit',
    minute: '2-digit',
    second: '2-digit',
  }).format(new Date(timestamp * 1000))
}

export function formatAuditInteger(value: number | undefined): string {
  return new Intl.NumberFormat().format(value ?? 0)
}

export function getAuditModeLabel(mode: SecurityAuditMode, t: TFunction) {
  switch (mode) {
    case 'blocking':
      return t('Blocking')
    case 'async_audit':
      return t('Async audit')
    default:
      return t('Off')
  }
}

export function SecurityAuditModeBadge({
  mode,
  t,
}: {
  mode: SecurityAuditMode
  t: TFunction
}) {
  return (
    <Badge
      variant={
        mode === 'blocking'
          ? 'destructive'
          : mode === 'async_audit'
            ? 'default'
            : 'secondary'
      }
    >
      {getAuditModeLabel(mode, t)}
    </Badge>
  )
}

export function DecisionBadge({
  decision,
  t,
}: {
  decision: string
  t: TFunction
}) {
  const normalized = decision.toLowerCase()
  const label =
    normalized === 'error'
      ? t('Error')
      : normalized === 'critical' || normalized === 'blocked'
        ? t('Blocked')
        : normalized === 'flag' || normalized === 'flagged'
          ? t('Flagged')
          : normalized === 'pass' ||
              normalized === 'allowed' ||
              normalized === 'safe'
            ? t('Allowed')
            : decision || t('Unknown')

  return (
    <Badge
      variant={
        normalized === 'critical' || normalized === 'blocked'
          ? 'destructive'
          : normalized === 'pass' ||
              normalized === 'allowed' ||
              normalized === 'safe'
            ? 'secondary'
            : 'outline'
      }
    >
      {label}
    </Badge>
  )
}
