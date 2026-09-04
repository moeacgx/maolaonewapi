/*
Copyright (C) 2025 QuantumNous

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
*/
import assert from 'node:assert/strict'
import fs from 'node:fs'
import test from 'node:test'

const hookSource = fs.readFileSync(
  new URL('./hooks/usage-logs/useUsageLogsData.jsx', import.meta.url),
  'utf8'
)
const columnsSource = fs.readFileSync(
  new URL('./components/table/usage-logs/UsageLogsColumnDefs.jsx', import.meta.url),
  'utf8'
)
const selectorSource = fs.readFileSync(
  new URL('./components/table/usage-logs/modals/ColumnSelectorModal.jsx', import.meta.url),
  'utf8'
)

test('Classic usage logs expose upstream response model only to admins', () => {
  assert.match(hookSource, /UPSTREAM_RESPONSE_MODEL:\s*'upstream_response_model'/)
  assert.match(hookSource, /\[COLUMN_KEYS\.UPSTREAM_RESPONSE_MODEL\]:\s*isAdminUser/)
  assert.match(columnsSource, /COLUMN_KEYS\.UPSTREAM_RESPONSE_MODEL/)
  assert.match(columnsSource, /upstream_response_model_name/)
  assert.match(selectorSource, /COLUMN_KEYS\.UPSTREAM_RESPONSE_MODEL/)
})
