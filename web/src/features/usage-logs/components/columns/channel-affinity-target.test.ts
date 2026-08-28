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
import { describe, expect, it } from 'vitest'

import { buildChannelAffinityUsageCacheTarget } from './channel-affinity-target'

describe('渠道亲和性详情目标映射', () => {
  it('优先使用 rule_name，并回退到历史 reason', () => {
    expect(
      buildChannelAffinityUsageCacheTarget({
        reason: 'legacy-rule',
        using_group: '',
        selected_group: 'default',
        key_hint: 'sess...tion',
        key_fp: 'a1b2c3d4',
      })
    ).toEqual({
      rule_name: 'legacy-rule',
      using_group: 'default',
      key_hint: 'sess...tion',
      key_fp: 'a1b2c3d4',
    })
  })

  it('不使用缺失 key_fp 的日志伪造详情身份', () => {
    expect(
      buildChannelAffinityUsageCacheTarget({
        reason: 'legacy-rule',
        key_hint: 'sess...tion',
      })
    ).toEqual({
      rule_name: 'legacy-rule',
      using_group: '',
      key_hint: 'sess...tion',
      key_fp: '',
    })
  })
})
