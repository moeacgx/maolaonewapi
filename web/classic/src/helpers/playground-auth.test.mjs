import assert from 'node:assert/strict';
import test from 'node:test';

import {
  createPlaygroundRequestHeaders,
  getCurrentAccessToken,
} from './playground-auth.js';

const createStorage = (user) => {
  let value = user ? JSON.stringify(user) : null;
  return {
    getItem: () => value,
    setUser(nextUser) {
      value = JSON.stringify(nextUser);
    },
  };
};

test('操练场请求头使用当前登录态中的 Bearer 令牌', () => {
  const storage = createStorage({ id: 7, token: 'access-token-current' });

  assert.equal(getCurrentAccessToken(storage), 'access-token-current');
  assert.deepEqual(createPlaygroundRequestHeaders('user-7', storage), {
    'Content-Type': 'application/json',
    'New-Api-User': 'user-7',
    Authorization: 'Bearer access-token-current',
  });
});

test('登录态替换后下一次请求读取新令牌而不是旧闭包值', () => {
  const storage = createStorage({ id: 7, token: 'access-token-before-login' });
  const headersBeforeLogin = createPlaygroundRequestHeaders('user-7', storage);

  storage.setUser({ id: 7, access_token: 'access-token-after-login' });
  const headersAfterLogin = createPlaygroundRequestHeaders('user-7', storage);

  assert.equal(
    headersBeforeLogin.Authorization,
    'Bearer access-token-before-login',
  );
  assert.equal(
    headersAfterLogin.Authorization,
    'Bearer access-token-after-login',
  );
});

test('无登录态时不伪造 Authorization 头', () => {
  const storage = createStorage(null);

  assert.deepEqual(createPlaygroundRequestHeaders('anonymous', storage), {
    'Content-Type': 'application/json',
    'New-Api-User': 'anonymous',
  });
});
