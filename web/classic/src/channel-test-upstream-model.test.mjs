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
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import test from 'node:test';

const modalSource = readFileSync(
  new URL(
    './components/table/channels/modals/ModelTestModal.jsx',
    import.meta.url,
  ),
  'utf8',
);
const hookSource = readFileSync(
  new URL('./hooks/channels/useChannelsData.jsx', import.meta.url),
  'utf8',
);

test('Classic 渠道测试展示上游响应模型并保存对应结果字段', () => {
  assert.match(hookSource, /upstream_response_model_name/);
  assert.match(modalSource, /title: t\('上游模型'\)/);
  assert.match(modalSource, /testResult\?\.upstream_response_model_name/);
});
