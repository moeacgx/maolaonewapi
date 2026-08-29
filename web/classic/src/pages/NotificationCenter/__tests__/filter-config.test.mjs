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
import test from 'node:test';
import { normalizeNotificationFilterConfig } from '../filter-config.js';

const notificationSource = fs.readFileSync(
  new URL('../index.jsx', import.meta.url),
  'utf8',
);
const filterSource = fs.readFileSync(
  new URL('../filter-config.js', import.meta.url),
  'utf8',
);
const classicCss = fs.readFileSync(
  new URL('../../../index.css', import.meta.url),
  'utf8',
);

test('保留状态码并去重多个报错关键词', () => {
  assert.deepEqual(
    normalizeNotificationFilterConfig({
      status_codes: ' 403, 500-599 ',
      error_keywords: ['余额不足', ' timeout ', '余额不足', ''],
    }),
    {
      status_codes: '403, 500-599',
      error_keywords: ['余额不足', 'timeout'],
    },
  );
});

test('空筛选不会写入无效配置', () => {
  assert.equal(
    normalizeNotificationFilterConfig({
      status_codes: ' ',
      error_keywords: [' ', ''],
    }),
    undefined,
  );
});

test('textarea 多行粘贴路径：split 后去空行，去重，再归一化', () => {
  const rawLines =
    '余额不足\r\n timeout \r\n\r\n余额不足\r\nQUOTA\r\nquota\r\n'.split(
      /\r?\n/,
    );
  assert.equal(rawLines.some((line) => line.includes('\r')), false);
  assert.deepEqual(
    normalizeNotificationFilterConfig({
      error_keywords: rawLines,
    }),
    {
      error_keywords: ['余额不足', 'timeout', 'QUOTA'],
    },
  );
});

test('关键词身份比较不依赖 locale，并保留首个原始拼写', () => {
  const originalToLocaleLowerCase = String.prototype.toLocaleLowerCase;
  String.prototype.toLocaleLowerCase = () => {
    throw new Error('locale-sensitive lower-case must not be used');
  };
  try {
    assert.deepEqual(
      normalizeNotificationFilterConfig({
        error_keywords: [' QUOTA ', 'quota', 'Timeout', ' timeout '],
      }),
      { error_keywords: ['QUOTA', 'Timeout'] },
    );
  } finally {
    String.prototype.toLocaleLowerCase = originalToLocaleLowerCase;
  }
  assert.doesNotMatch(filterSource, /toLocaleLowerCase/);
  assert.match(filterSource, /keyword\.toLowerCase\(\)/);
});

test('TextArea 使用 CRLF 安全的逐行拆分', () => {
  assert.match(notificationSource, /value\.split\(\/\\r\?\\n\/\)/);
});

test('非 channel_disabled 任务保存 payload 不包含 filter_config', () => {
  assert.doesNotMatch(notificationSource, /^\s*filter_config: filterConfig,\s*$/m);
  assert.match(
    notificationSource,
    /\.\.\.\(\s*taskForm\.event_type === CHANNEL_DISABLED_EVENT\s*\?\s*\{\s*filter_config: filterConfig\s*\}\s*:\s*\{\}\s*\)/s,
  );
});

test('任务 Modal 和窄屏 CSS 使用专用作用域', () => {
  assert.match(notificationSource, /className='classic-notification-task-modal'/);
  assert.match(
    notificationSource,
    /className='classic-notification-target-card[^']*'/,
  );
  assert.match(
    notificationSource,
    /bodyStyle=\{\{[\s\S]*maxHeight: 'calc\(100vh - \d+px\)'[\s\S]*overflowY: 'auto'[\s\S]*overflowX: 'hidden'[\s\S]*\}\}/,
  );
  assert.doesNotMatch(classicCss, /\.semi-modal-wrap\s+\.semi-modal\s*\{/);
  assert.doesNotMatch(
    classicCss,
    /\.classic-notification-task-body\s+\.semi-taginput/,
  );
  assert.match(
    classicCss,
    /\.classic-notification-task-modal[\s\S]*\.semi-tagInput/,
  );
  assert.match(
    classicCss,
    /\.classic-notification-task-modal[\s\S]*\.classic-notification-target-card/,
  );
});
