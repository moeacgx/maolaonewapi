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
import type { StatusVariant } from '@/components/status-badge'

import type { InvoiceStatus } from './types'

export type InvoiceStatusPresentation = {
  labelKey: 'Pending payment' | 'Pending issue' | 'Issued' | 'Closed'
  variant: StatusVariant
}

/** Stable UI contract shared by user and administrator invoice lists. */
export function getInvoiceStatusPresentation(
  status: InvoiceStatus
): InvoiceStatusPresentation {
  switch (status) {
    case 'payment_pending':
      return { labelKey: 'Pending payment', variant: 'warning' }
    case 'issued':
      return { labelKey: 'Issued', variant: 'success' }
    case 'closed':
      return { labelKey: 'Closed', variant: 'neutral' }
    case 'pending':
      return { labelKey: 'Pending issue', variant: 'warning' }
  }
}
