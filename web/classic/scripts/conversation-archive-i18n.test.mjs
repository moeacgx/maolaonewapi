import assert from 'node:assert/strict';
import { readFile } from 'node:fs/promises';
import path from 'node:path';
import test from 'node:test';
import { fileURLToPath } from 'node:url';

const scriptDir = path.dirname(fileURLToPath(import.meta.url));
const classicRoot = path.resolve(scriptDir, '..');
const repositoryRoot = path.resolve(classicRoot, '../..');
const nativeEntry = path.resolve(
  classicRoot,
  '../../extension/builtin/conversation-archive/public/native/classic.mjs',
);
const localeNames = ['en', 'zh', 'zh-CN', 'zh-TW', 'fr', 'ja', 'ru', 'vi'];

async function assertNativeEntryIsLocalized(entryPath, localesDirectory, locales, templateName) {
  const source = await readFile(entryPath, 'utf8');
  const keys = new Set(
    [...source.matchAll(/\bt\('([^']+)'/g)].map((match) => match[1]),
  );
  for (const key of ['Created', 'User', 'Group', 'Model', 'Protocol', 'Messages', 'Size']) {
    keys.add(key);
  }

  for (const localeName of locales) {
    const localePath = path.join(localesDirectory, `${localeName}.json`);
    const locale = JSON.parse(await readFile(localePath, 'utf8'));
    const missing = [...keys].filter(
      (key) => !Object.prototype.hasOwnProperty.call(locale.translation, key),
    );
    assert.deepEqual(missing, [], `${templateName} 的 ${localeName} 缺少对话归档翻译键`);
    if (localeName === 'zh' || localeName === 'zh-CN' || localeName === 'zh-TW') {
      const untranslated = [...keys].filter((key) => locale.translation[key] === key);
      assert.deepEqual(
        untranslated,
        [],
        `${templateName} 的 ${localeName} 对话归档页面不能回退为英文原文`,
      );
    }
  }
}

test('Default 与 Classic 对话归档页面的全部文案均有本地化', async () => {
  await assertNativeEntryIsLocalized(
    nativeEntry,
    path.join(classicRoot, 'src/i18n/locales'),
    localeNames,
    'Classic',
  );
  await assertNativeEntryIsLocalized(
    path.join(repositoryRoot, 'extension/builtin/conversation-archive/public/native/default.mjs'),
    path.join(repositoryRoot, 'web/src/i18n/locales'),
    ['en', 'zh', 'zh-TW', 'fr', 'ja', 'ru', 'vi'],
    'Default',
  );
});
