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
import { readFileSync } from 'node:fs'
import { dirname, resolve } from 'node:path'
import test from 'node:test'
import { fileURLToPath } from 'node:url'
import { cleanSecurityAuditEventFilter } from './event-filter.ts'

const root = dirname(fileURLToPath(import.meta.url))
const readSource = (file: string) => readFileSync(resolve(root, file), 'utf8')

test('审计事件渠道筛选进入统一列表和删除筛选契约', () => {
  assert.deepEqual(cleanSecurityAuditEventFilter({ channel_id: 42 }), {
    channel_id: 42,
  })
  assert.deepEqual(cleanSecurityAuditEventFilter({ channel_id: 0 }), {})

  const types = readSource('types.ts')
  const view = readSource('events-view.tsx')
  assert.match(types, /channel_id\?:\s*number/)
  assert.match(view, /draftFilter\.channel_id/)
  assert.match(view, /DataTableViewOptions table=\{table\}/)
})

test('Default 与 Classic 列表都展示中文化风险字段和拦截关键词', () => {
  const defaultView = readSource('events-view.tsx')
  const classicView = readSource(
    '../../../../classic/src/pages/SecurityAudit/EventsTab.jsx'
  )

  assert.match(defaultView, /eventRiskLevelLabel/)
  assert.match(defaultView, /eventCategoryLabel/)
  assert.match(defaultView, /sensitive_word:\s*'Sensitive words'/)
  assert.match(
    defaultView,
    /cyber_policy:\s*'Official risk control \(cyber_policy\)'/
  )
  assert.match(defaultView, /Blocked keywords/)
  assert.match(classicView, /getDecisionLabel/)
  assert.match(classicView, /getRiskLevelLabel/)
  assert.match(classicView, /getCategoryLabel/)
  assert.match(classicView, /sensitive_word:\s*'屏蔽词'/)
  assert.match(classicView, /cyber_policy:\s*'官方风控（cyber_policy）'/)
  assert.match(classicView, /拦截关键词/)
  assert.match(classicView, /columnVisibility/)
})
