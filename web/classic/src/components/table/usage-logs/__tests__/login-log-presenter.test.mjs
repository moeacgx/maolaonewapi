import assert from 'node:assert/strict';
import test from 'node:test';

import {
  getLoginLogDetailItems,
  getLoginLogSummary,
  getLoginMethodLabel,
  LOG_TYPE_LOGIN,
} from '../login-log-presenter.js';

const translations = {
  IP: 'IP',
  'User Agent': 'User Agent',
  两步验证: '两步验证',
  密码: '密码',
  微信: '微信',
  登录信息: '登录信息',
  登录成功: '登录成功',
  '登录成功（通过 {{method}}）': '登录成功（通过 {{method}}）',
  登录方式: '登录方式',
  未知: '未知',
};

const t = (key, vars = {}) => {
  let text = translations[key] || key;
  for (const [name, value] of Object.entries(vars)) {
    text = text.replace(`{{${name}}}`, value);
  }
  return text;
};

test('Classic 使用日志将 type=7 密码登录展示为登录审计', () => {
  const log = {
    type: LOG_TYPE_LOGIN,
    content: 'Logged in successfully via password',
    ip: '127.0.0.1',
  };
  const other = {
    login_method: 'password',
    user_agent: 'Chrome',
    op: {
      action: 'login',
      params: { method: 'password' },
    },
  };

  assert.equal(getLoginLogSummary(log, other, t), '登录成功（通过 密码）');
  assert.deepEqual(getLoginLogDetailItems(log, other, t), [
    { key: '登录信息', value: '登录成功（通过 密码）' },
    { key: '登录方式', value: '密码' },
    { key: 'IP', value: '127.0.0.1' },
    { key: 'User Agent', value: 'Chrome' },
  ]);
});

test('Classic 使用日志保留 OAuth 登录方式提供商标识', () => {
  assert.equal(getLoginMethodLabel('oauth:github', t), 'OAuth github');
  assert.equal(getLoginMethodLabel('oauth', t), 'OAuth');
});

test('Classic 使用日志支持旧登录内容回退并忽略非登录日志', () => {
  assert.equal(
    getLoginLogSummary(
      { type: LOG_TYPE_LOGIN, content: 'Logged in successfully via passkey' },
      {},
      t,
    ),
    '登录成功（通过 Passkey）',
  );
  assert.equal(getLoginLogSummary({ type: 2, content: 'consume' }, {}, t), null);
  assert.deepEqual(getLoginLogDetailItems({ type: 2 }, {}, t), []);
});
