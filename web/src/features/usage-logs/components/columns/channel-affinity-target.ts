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

For commercial licensing, please contact support@quantumnous.com
*/
import type { ChannelAffinityInfo } from '../../types'

export interface ChannelAffinityUsageCacheTarget {
  rule_name: string
  using_group: string
  key_hint: string
  key_fp: string
}

export function buildChannelAffinityUsageCacheTarget(
  affinity: ChannelAffinityInfo
): ChannelAffinityUsageCacheTarget {
  return {
    rule_name: affinity.rule_name || affinity.reason || '',
    using_group: affinity.using_group || affinity.selected_group || '',
    key_hint: affinity.key_hint || '',
    key_fp: affinity.key_fp || '',
  }
}
