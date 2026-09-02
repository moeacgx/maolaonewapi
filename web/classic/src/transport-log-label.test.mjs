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
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import test from 'node:test';

import { getLogTransportLabel } from './helpers/log.js';

const t = (key) => key;

test('Classic 使用日志显示稳定的传输标签', () => {
  assert.equal(
    getLogTransportLabel({ transport: 'websocket' }, t),
    'WebSocket',
  );
  assert.equal(getLogTransportLabel({ transport: 'http' }, t), 'HTTP/SSE');
  assert.equal(getLogTransportLabel({}, t), '未知');
  assert.equal(getLogTransportLabel({ transport: 'internal_code' }, t), '未知');
});

test('Classic 系统设置接入 Responses WebSocket 布尔选项', () => {
  const source = readFileSync(
    new URL('./components/settings/SystemSetting.jsx', import.meta.url),
    'utf8',
  );
  assert.match(source, /ResponsesWebsocketEnabled:\s*false/);
  assert.match(source, /case 'ResponsesWebsocketEnabled':[\s\S]*?toBoolean/);
  assert.match(
    source,
    /handleCheckboxChange\('ResponsesWebsocketEnabled', e\)/,
  );
});
