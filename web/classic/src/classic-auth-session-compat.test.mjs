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

For commercial licensing, please contact support@quantumnous.com
*/

import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import test from 'node:test';

import { normalizeAuthData } from './helpers/auth-data.js';

const readSource = (path) => readFileSync(new URL(path, import.meta.url), 'utf8');

test('normalizes session auth bundle for Classic user state', () => {
  const normalized = normalizeAuthData({
    access_token: 'access-token',
    access_expires_at: 123,
    session: { id: 'session-id' },
    user: { id: 7, role: 100, username: 'root' },
  });

  assert.deepEqual(normalized, {
    id: 7,
    role: 100,
    username: 'root',
    token: 'access-token',
    access_token: 'access-token',
    access_expires_at: 123,
    session: { id: 'session-id' },
  });
});

test('keeps legacy Classic login responses compatible', () => {
  assert.deepEqual(normalizeAuthData({ id: 7, role: 100, token: 'legacy' }), {
    id: 7,
    role: 100,
    token: 'legacy',
    access_token: 'legacy',
  });
});

test('uses current logout, refresh, bearer and 2FA flow contracts', () => {
  const apiSource = readSource('./helpers/api.js');
  const loginSource = readSource('./components/auth/LoginForm.jsx');
  const twoFASource = readSource('./components/auth/TwoFAVerification.jsx');

  assert.match(apiSource, /post\('\/api\/user\/auth\/logout'/);
  assert.doesNotMatch(apiSource, /get\('\/api\/user\/logout'/);
  assert.match(apiSource, /post\('\/api\/user\/auth\/refresh'/);
  assert.match(apiSource, /Authorization: `Bearer \$\{token\}`/);
  assert.match(loginSource, /data\.flow_token/);
  assert.match(twoFASource, /flow_token: flowToken/);
});
