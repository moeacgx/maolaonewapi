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

test('安全审计事件使用用户名文本筛选且不再提交用户 ID', () => {
  const events = readSource('events-view.tsx')
  const types = readSource('types.ts')
  const filterContract = types.slice(
    types.indexOf('export interface SecurityAuditEventFilter'),
    types.indexOf('export interface SecurityAuditBuiltinPolicy')
  )

  assert.match(filterContract, /username\?:\s*string/)
  assert.doesNotMatch(filterContract, /user_id\?:/)
  assert.match(events, /id='audit-event-username'/)
  assert.match(events, /value=\{draftFilter\.username \?\? ''\}/)
  assert.match(events, /maxLength=\{128\}/)
  assert.match(events, /username:\s*event\.target\.value/)
  assert.doesNotMatch(events, /draftFilter\.user_id/)
})

test('用户名筛选去除首尾空格并忽略空值', () => {
  assert.deepEqual(
    cleanSecurityAuditEventFilter({
      username: '  audit-reviewer  ',
      decision: ' flag ',
      token_id: 0,
    }),
    {
      username: 'audit-reviewer',
      decision: 'flag',
    }
  )
  assert.deepEqual(cleanSecurityAuditEventFilter({ username: '   ' }), {})
})
