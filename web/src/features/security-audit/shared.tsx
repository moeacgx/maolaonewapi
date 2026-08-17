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
import { getAuditModeLabel } from './utils'

export function SecurityAuditModeBadge({
  mode,
  t,
}: {
  mode: SecurityAuditMode
  t: TFunction
}) {
  let variant: 'destructive' | 'default' | 'secondary' = 'secondary'
  if (mode === 'blocking') variant = 'destructive'
  else if (mode === 'async_audit') variant = 'default'

  return <Badge variant={variant}>{getAuditModeLabel(mode, t)}</Badge>
}

export function DecisionBadge({
  decision,
  t,
}: {
  decision: string
  t: TFunction
}) {
  const normalized = decision.toLowerCase()
  let label = decision || t('Unknown')
  if (normalized === 'error') label = t('Error')
  else if (normalized === 'critical' || normalized === 'blocked') {
    label = t('Blocked')
  } else if (normalized === 'flag' || normalized === 'flagged') {
    label = t('Flagged')
  } else if (
    normalized === 'pass' ||
    normalized === 'allowed' ||
    normalized === 'safe'
  ) {
    label = t('Allowed')
  }

  let variant: 'destructive' | 'secondary' | 'outline' = 'outline'
  if (normalized === 'critical' || normalized === 'blocked') {
    variant = 'destructive'
  } else if (
    normalized === 'pass' ||
    normalized === 'allowed' ||
    normalized === 'safe'
  ) {
    variant = 'secondary'
  }

  return <Badge variant={variant}>{label}</Badge>
}
