import assert from 'node:assert/strict';
import test from 'node:test';

import {
  buildCCSwitchURL,
  isCCSwitchPreset,
  isValidCCSwitchAddress,
  normalizeCCSwitchAddress,
  resolveCCSwitchAddresses,
  syncCCSwitchAPIAddressToStorage,
  withCCSwitchAPIAddress,
} from './ccSwitch.js';

function parseImportURL(url) {
  const query = url.slice(url.indexOf('?') + 1);
  return new URLSearchParams(query);
}

test('CC Switch API 地址去除空白和末尾斜杠', () => {
  assert.equal(
    normalizeCCSwitchAddress('  https://api.example.com///  '),
    'https://api.example.com',
  );
});

test('保存后立即覆盖本地状态中的旧地址', () => {
  let stored = JSON.stringify({
    cc_switch_api_address: 'https://old.example.com',
    server_address: 'https://www.example.com',
  });
  const storage = {
    getItem: () => stored,
    setItem: (_, value) => {
      stored = value;
    },
  };

  const updated = syncCCSwitchAPIAddressToStorage(
    '  https://new.example.com/gateway///  ',
    storage,
  );

  assert.equal(
    updated.cc_switch_api_address,
    'https://new.example.com/gateway',
  );
  assert.equal(
    JSON.parse(stored).cc_switch_api_address,
    'https://new.example.com/gateway',
  );
});

test('清空自定义地址时覆盖旧值并回退网站地址', () => {
  const updated = withCCSwitchAPIAddress(
    {
      cc_switch_api_address: 'https://old.example.com',
      server_address: 'https://www.example.com',
    },
    '   ',
  );

  assert.equal(updated.cc_switch_api_address, '');
  assert.deepEqual(resolveCCSwitchAddresses(updated), {
    apiAddress: 'https://www.example.com',
    homepage: 'https://www.example.com',
  });
});

test('Classic 大小写不敏感识别 ccswitch 聊天入口', () => {
  assert.equal(isCCSwitchPreset('ccswitch'), true);
  assert.equal(isCCSwitchPreset('  CCSwitch://open'), true);
  assert.equal(isCCSwitchPreset('https://ccswitch.io'), false);
});

test('CC Switch API 地址仅接受无凭据和附加参数的 HTTP 绝对地址', () => {
  assert.equal(isValidCCSwitchAddress(''), true);
  assert.equal(isValidCCSwitchAddress('https://api.example.com/gateway'), true);
  assert.equal(isValidCCSwitchAddress('api.example.com'), false);
  assert.equal(isValidCCSwitchAddress('ftp://api.example.com'), false);
  assert.equal(isValidCCSwitchAddress('https://user:pass@example.com'), false);
  assert.equal(
    isValidCCSwitchAddress('https://api.example.com?token=x'),
    false,
  );
});

test('自定义 API 地址优先且 homepage 始终保留网站地址', () => {
  assert.deepEqual(
    resolveCCSwitchAddresses(
      {
        cc_switch_api_address: 'https://api.example.com/',
        server_address: 'https://www.example.com/',
      },
      'https://fallback.example.com',
    ),
    {
      apiAddress: 'https://api.example.com',
      homepage: 'https://www.example.com',
    },
  );
});

test('空配置依次回退网站地址和当前页面来源', () => {
  assert.deepEqual(
    resolveCCSwitchAddresses(
      { cc_switch_api_address: '', server_address: 'https://www.example.com/' },
      'https://fallback.example.com',
    ),
    {
      apiAddress: 'https://www.example.com',
      homepage: 'https://www.example.com',
    },
  );
  assert.deepEqual(
    resolveCCSwitchAddresses({}, 'https://fallback.example.com/'),
    {
      apiAddress: 'https://fallback.example.com',
      homepage: 'https://fallback.example.com',
    },
  );
});

test('Codex 幂等补充 /v1，Claude 和 Gemini 使用 API 根地址', () => {
  const common = {
    name: 'Provider',
    models: { model: 'gpt-test' },
    apiKey: 'sk-test',
    status: {
      cc_switch_api_address: 'https://api.example.com/',
      server_address: 'https://www.example.com/',
    },
    origin: 'https://fallback.example.com',
  };

  const codex = parseImportURL(buildCCSwitchURL({ ...common, app: 'codex' }));
  const claude = parseImportURL(buildCCSwitchURL({ ...common, app: 'claude' }));
  const gemini = parseImportURL(buildCCSwitchURL({ ...common, app: 'gemini' }));

  assert.equal(codex.get('endpoint'), 'https://api.example.com/v1');
  assert.equal(claude.get('endpoint'), 'https://api.example.com');
  assert.equal(gemini.get('endpoint'), 'https://api.example.com');
  assert.equal(codex.get('homepage'), 'https://www.example.com');

  const existingV1 = parseImportURL(
    buildCCSwitchURL({
      ...common,
      app: 'codex',
      status: {
        ...common.status,
        cc_switch_api_address: 'https://api.example.com/v1/',
      },
    }),
  );
  assert.equal(existingV1.get('endpoint'), 'https://api.example.com/v1');
});
