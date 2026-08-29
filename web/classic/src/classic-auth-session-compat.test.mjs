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

import { getAuthErrorMessage, normalizeAuthData } from './helpers/auth-data.js';

const readSource = (path) =>
  readFileSync(new URL(path, import.meta.url), 'utf8');

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

test('uses current logout, refresh, bearer, OAuth, 2FA, and Passkey contracts', () => {
  const apiSource = readSource('./helpers/api.js');
  const loginSource = readSource('./components/auth/LoginForm.jsx');
  const twoFASource = readSource('./components/auth/TwoFAVerification.jsx');
  const oauthCallbackSource = readSource(
    './components/auth/OAuth2Callback.jsx',
  );
  const registerSource = readSource('./components/auth/RegisterForm.jsx');
  const personalSource = readSource(
    './components/settings/PersonalSetting.jsx',
  );
  const secureSource = readSource('./services/secureVerification.js');
  const playgroundSource = readSource('./hooks/playground/useApiRequest.jsx');

  assert.match(apiSource, /post\('\/api\/user\/auth\/logout'/);
  assert.doesNotMatch(apiSource, /get\('\/api\/user\/logout'/);
  assert.match(apiSource, /post\('\/api\/user\/auth\/refresh'/);
  assert.match(apiSource, /session\?\.sid \|\| session\?\.id/);
  assert.match(apiSource, /Authorization: `Bearer \$\{token\}`/);
  assert.match(apiSource, /post\(\s*'\/api\/oauth\/state',\s*\{/s);
  assert.match(apiSource, /\/api\/oauth\/state'[\s\S]*skipErrorHandler: true/);
  assert.match(apiSource, /getAuthErrorMessage\(error, t\) \|\| error/);
  assert.match(apiSource, /provider,\s+intent,\s+aff:/);
  assert.match(apiSource, /prepareOAuthState\(options, 'github'\)/);
  assert.match(apiSource, /prepareOAuthState\(options, 'discord'\)/);
  assert.match(apiSource, /prepareOAuthState\(options, 'oidc'\)/);
  assert.match(apiSource, /prepareOAuthState\(options, 'linuxdo'\)/);
  assert.match(apiSource, /prepareOAuthState\(options, provider\.slug\)/);

  assert.match(loginSource, /data\.flow_token/);
  assert.match(loginSource, /flow_token: flowToken,\s+credential: payload,/s);
  assert.match(loginSource, /normalizeAuthData\(finish\.data\)/);
  assert.match(loginSource, /oauth\/wechat\?code=.*skipErrorHandler/s);
  assert.match(loginSource, /oauth\/telegram\/login.*skipErrorHandler/s);
  assert.match(loginSource, /passkey\/login\/begin.*skipErrorHandler/s);
  assert.match(loginSource, /passkey\/login\/finish.*skipErrorHandler/s);
  assert.match(registerSource, /oauth\/wechat\?code=.*skipErrorHandler/s);
  assert.match(registerSource, /oauth\/telegram\/login.*skipErrorHandler/s);
  assert.match(twoFASource, /flow_token: flowToken/);
  assert.match(twoFASource, /skipErrorHandler: true/);
  assert.match(twoFASource, /getAuthErrorMessage\(error,\s*t\)/);
  assert.match(oauthCallbackSource, /skipErrorHandler: true/);
  assert.match(oauthCallbackSource, /getAuthErrorMessage\(error, t\)/);
  assert.match(oauthCallbackSource, /navigate\('\/login'\)/);
  assert.match(twoFASource, /normalizeAuthData\(res\.data\.data\)/);

  assert.match(personalSource, /flow_token: flowToken, credential: payload/);
  assert.match(personalSource, /getProofHeaders\(\s+'passkey\.register'/s);
  assert.match(
    secureSource,
    /method: '2fa',\s+code: code\.trim\(\),\s+scope,/s,
  );
  assert.match(
    secureSource,
    /\/api\/user\/passkey\/verify\/begin', \{\s+scope,/s,
  );
  assert.match(
    secureSource,
    /\/api\/user\/passkey\/verify\/finish', \{\s+flow_token: flowToken,\s+credential: assertionResult,/s,
  );
  assert.match(secureSource, /X-Security-Proof/);
  assert.doesNotMatch(secureSource, /method: 'passkey'/);
  assert.equal(
    playgroundSource.match(/headers:\s*createPlaygroundRequestHeaders/g)
      ?.length,
    2,
  );
});

test('uses one batch request when loading Classic token keys', () => {
  const tokenSource = readSource('./helpers/token.js');

  assert.match(tokenSource, /fetchTokenKeysBatch\(\s*activeTokens\.map\(/s);
  assert.doesNotMatch(
    tokenSource,
    /Promise\.allSettled\(\s*activeTokens\.map\(\(token\) => fetchTokenKey/s,
  );
});

test('maps AUTH_SESSION_LIMIT to safe recovery guidance', () => {
  const message = getAuthErrorMessage(
    { response: { data: { code: 'AUTH_SESSION_LIMIT' } } },
    (key) =>
      key === 'AUTH_SESSION_LIMIT'
        ? 'active sessions are full; use password reset when no signed-in device is available'
        : key,
  );

  assert.match(message, /^AUTH_SESSION_LIMIT:/);
  assert.match(message, /password reset/);
});

test('keeps session-limit recovery text reachable in every Classic locale', () => {
  const localeFiles = [
    'en.json',
    'fr.json',
    'ja.json',
    'ru.json',
    'vi.json',
    'zh-CN.json',
    'zh-TW.json',
    'zh.json',
  ];

  for (const localeFile of localeFiles) {
    const locale = JSON.parse(readSource(`./i18n/locales/${localeFile}`));
    const message = locale.translation.AUTH_SESSION_LIMIT;
    assert.equal(typeof message, 'string', `${localeFile} defines the key`);
    assert.match(
      message,
      /邮箱|电子邮件|電子郵件|email|e-mail|correo|почт|メール/i,
    );
    assert.doesNotMatch(
      message,
      /已登录设备|signed-in device|open Login sessions|sessions de connexion et déconnectez|ログイン済み端末で|авторизованном устройстве откройте|đã đăng nhập.*đăng xuất|已登入裝置的|登入工作階段[」”"]中/i,
    );
  }
});

test('keeps unknown login errors available to the generic fallback', () => {
  assert.equal(
    getAuthErrorMessage(
      { response: { data: { code: 'UNKNOWN_LOGIN_ERROR' } } },
      (key) => key,
    ),
    null,
  );
});

test('handles session-limit login errors locally without duplicate interceptor toasts', () => {
  const loginSource = readSource('./components/auth/LoginForm.jsx');

  assert.match(loginSource, /skipErrorHandler:\s*true/);
  assert.match(loginSource, /getAuthErrorMessage\(error, t\)/);
});
