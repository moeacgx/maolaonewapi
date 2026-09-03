import assert from 'node:assert/strict';
import test from 'node:test';

import { isWebSocketLog } from './helpers/log.js';

test('Classic 使用日志仅将 ws=true 识别为 WebSocket', () => {
  assert.equal(isWebSocketLog({ ws: true }), true);
  assert.equal(isWebSocketLog({ ws: false }), false);
  assert.equal(isWebSocketLog({ ws: 'true' }), false);
  assert.equal(isWebSocketLog({}), false);
  assert.equal(isWebSocketLog(null), false);
});
