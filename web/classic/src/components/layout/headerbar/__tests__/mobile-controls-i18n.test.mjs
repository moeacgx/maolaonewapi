import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import test from 'node:test';

const localeFiles = [
  'en.json',
  'fr.json',
  'ja.json',
  'ru.json',
  'vi.json',
  'zh-CN.json',
  'zh-TW.json',
];

const controlKeys = ['common.changeLanguage', '通知中心', '切换导航菜单'];

test('mobile header controls keep accessible labels translated in every locale', () => {
  for (const localeFile of localeFiles) {
    const locale = JSON.parse(
      readFileSync(
        new URL(`../../../../i18n/locales/${localeFile}`, import.meta.url),
      ),
    );

    for (const key of controlKeys) {
      const value = locale.translation?.[key];
      assert.equal(
        typeof value,
        'string',
        `${localeFile} must define the ${key} translation`,
      );
      assert.notEqual(
        value.trim(),
        '',
        `${localeFile} must not leave ${key} empty`,
      );
      if (key === 'common.changeLanguage') {
        assert.notEqual(
          value,
          key,
          `${localeFile} must not expose the ${key} key as its label`,
        );
      }
    }
  }
});
