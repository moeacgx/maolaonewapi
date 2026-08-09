import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { dirname, resolve } from 'node:path';
import test from 'node:test';
import { fileURLToPath } from 'node:url';

const root = dirname(fileURLToPath(import.meta.url));
const readSource = (...parts) => readFileSync(resolve(root, ...parts), 'utf8');

test('Classic 令牌按钮和聊天 ccswitch 入口共用同一个弹窗', () => {
  const hookSource = readSource('hooks/tokens/useTokensData.jsx');
  const pageSource = readSource('components/table/tokens/index.jsx');

  assert.match(
    hookSource,
    /const openCCSwitchForRecord = async[\s\S]*?openCCSwitchModal\(fullKey\)/,
  );
  assert.match(
    hookSource,
    /isCCSwitchPreset\(url\)[\s\S]*?openCCSwitchModal\(fullKey\)/,
  );
  assert.match(pageSource, /useTokensData\([\s\S]*?openCCSwitchModalRef/);
  assert.equal((pageSource.match(/<CCSwitchModal/g) || []).length, 1);
});

test('Classic 聊天设置保存后立即同步公开状态缓存', () => {
  const source = readSource('pages/Setting/Chat/SettingsChats.jsx');

  assert.match(source, /field='CCSwitchAPIAddress'/);
  assert.match(
    source,
    /item\.key === 'CCSwitchAPIAddress'[\s\S]*?syncCCSwitchAPIAddressToStorage\([\s\S]*?submitInputs\.CCSwitchAPIAddress/,
  );
  assert.doesNotMatch(source, /API\.get\('\/api\/status'\)/);
});
