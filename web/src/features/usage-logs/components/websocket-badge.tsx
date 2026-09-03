/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as published by
the Free Software Foundation, either version 3 of the License, or
(at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program. If not, see <https://www.gnu.org/licenses/>.
*/
import { Radio } from 'lucide-react'
import { useTranslation } from 'react-i18next'

import { StatusBadge } from '@/components/status-badge'

/** 使用日志中的 WebSocket 请求标记。 */
export function WebSocketBadge() {
  const { t } = useTranslation()

  return (
    <StatusBadge
      label='WS'
      icon={Radio}
      variant='blue'
      size='sm'
      showDot={false}
      copyable={false}
      title={t('WebSocket')}
      aria-label={t('WebSocket')}
      className='border border-info/30 bg-info/10 font-mono text-info'
    />
  )
}
