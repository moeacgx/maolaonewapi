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
import fs from 'node:fs';
import path from 'node:path';
import { fileURLToPath } from 'node:url';

const testDir = path.dirname(fileURLToPath(import.meta.url));
const localesDir = path.resolve(testDir, '../../../../../i18n/locales');
const locales = ['zh-CN', 'zh-TW', 'en', 'fr', 'ru', 'ja', 'vi'];
const keys = [
  '充值优惠',
  '充值单价 = 官方美元单价 ×（充值汇率 ÷ 美元汇率）× 分组倍率 × 展示货币汇率',
];

for (const locale of locales) {
  const messages = JSON.parse(
    fs.readFileSync(path.join(localesDir, `${locale}.json`), 'utf8'),
  ).translation;

  for (const key of keys) {
    assert.equal(
      typeof messages[key],
      'string',
      `${locale} 缺少计费说明文案：${key}`,
    );
    assert.notEqual(messages[key].trim(), '', `${locale} 的 ${key} 不能为空`);
  }
}

console.log('billing guide i18n tests passed');
