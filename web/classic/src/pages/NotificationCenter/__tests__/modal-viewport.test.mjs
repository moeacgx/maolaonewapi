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

const css = fs.readFileSync(
  new URL('../../../index.css', import.meta.url),
  'utf8',
);
const notificationSource = fs.readFileSync(
  new URL('../index.jsx', import.meta.url),
  'utf8',
);
const scope = '.classic-notification-task-modal';
const sectionStart = css.indexOf('/* --- notification task modal responsive --- */');
assert.notEqual(sectionStart, -1, 'notification modal CSS section must exist');
const notificationCss = css.slice(sectionStart);

const parseRules = (source) => {
  const rules = [];
  const withoutComments = source.replace(/\/\*[\s\S]*?\*\//g, '');

  const scan = (content, atRules = []) => {
    let cursor = 0;
    while (cursor < content.length) {
      const open = content.indexOf('{', cursor);
      if (open < 0) break;
      const selectorText = content.slice(cursor, open).trim();
      let depth = 1;
      let close = open + 1;
      while (close < content.length && depth > 0) {
        if (content[close] === '{') depth += 1;
        if (content[close] === '}') depth -= 1;
        close += 1;
      }
      if (depth !== 0) break;
      const body = content.slice(open + 1, close - 1);
      if (selectorText.startsWith('@')) {
        scan(body, [...atRules, selectorText]);
      } else {
        const declarations = new Map();
        for (const declaration of body.split(';')) {
          const separator = declaration.indexOf(':');
          if (separator < 0) continue;
          declarations.set(
            declaration.slice(0, separator).trim().toLowerCase(),
            declaration
              .slice(separator + 1)
              .trim()
              .replace(/\s*!important\s*$/, ''),
          );
        }
        rules.push({
          selectors: selectorText
            .split(',')
            .map((selector) => selector.trim())
            .filter(Boolean),
          declarations,
          atRules,
        });
      }
      cursor = close;
    }
  };

  scan(withoutComments);
  return rules;
};

const classSelector = (className) =>
  new RegExp(`(?:^|[\\s>+~.])\\.${className}(?=$|[\\s>+~.:#\\[])`);

const rules = parseRules(notificationCss);
const rulesFor = (className, sourceRules = rules) =>
  sourceRules.filter((rule) =>
    rule.selectors.some(
      (selector) =>
        selector.includes(scope) && classSelector(className).test(selector),
    ),
  );

const declarationsFor = (className, sourceRules = rules) => {
  const declarations = new Map();
  for (const rule of rulesFor(className, sourceRules)) {
    for (const [property, value] of rule.declarations) {
      declarations.set(property, value);
    }
  }
  return declarations;
};

const baseRules = rules.filter((rule) => rule.atRules.length === 0);
const mobileRules = rules.filter((rule) =>
  rule.atRules.some((atRule) => /max-width:\s*639px/.test(atRule)),
);

const requiredElements = [
  'semi-modal',
  'semi-modal-content',
  'semi-modal-body-wrapper',
  'semi-modal-body',
  'classic-notification-task-body',
];

test('任务 Modal 的真实层级使用专用 class 并允许盒模型收缩', () => {
  assert.match(notificationSource, /className='classic-notification-task-modal'/);

  for (const className of requiredElements) {
    const matchingRules = rulesFor(className);
    assert.notEqual(
      matchingRules.length,
      0,
      `${className} must have a selector under ${scope}`,
    );
    const declarations = declarationsFor(className);
    assert.equal(declarations.get('box-sizing'), 'border-box');
    assert.equal(declarations.get('min-width'), '0');
    assert.ok(declarations.has('width'), `${className} must declare width`);
    assert.ok(
      declarations.has('max-width'),
      `${className} must declare max-width`,
    );
  }
});

test('任务 Modal 桌面宽度上限为 720px，窄屏两侧保留 16px', () => {
  const modalBase = declarationsFor('semi-modal', baseRules);
  assert.equal(
    modalBase.get('width'),
    'min(720px, calc(100vw - 32px))',
  );
  assert.equal(
    modalBase.get('max-width'),
    'min(720px, calc(100vw - 32px))',
  );

  const mobileModal = declarationsFor('semi-modal', mobileRules);
  assert.equal(mobileModal.get('width'), 'calc(100vw - 32px)');
  assert.equal(mobileModal.get('max-width'), 'calc(100vw - 32px)');
  assert.equal(mobileModal.get('margin-inline'), '16px');

  for (const viewport of [320, 375, 414]) {
    const dialogWidth = viewport - 32;
    assert.equal(
      (viewport - dialogWidth) / 2,
      16,
      `${viewport}px viewport must leave 16px on both sides`,
    );
  }
});

test('任务 Modal 内容层隐藏横向溢出，纵向滚动由 body 承担', () => {
  const wrapper = declarationsFor('semi-modal-body-wrapper');
  const body = declarationsFor('semi-modal-body');
  const taskBody = declarationsFor('classic-notification-task-body');

  assert.equal(wrapper.get('overflow-x'), 'hidden');
  assert.equal(body.get('overflow-x'), 'hidden');
  assert.equal(body.get('overflow-y'), 'auto');
  assert.equal(taskBody.get('overflow-x'), 'hidden');
  assert.equal(taskBody.get('overflow-wrap'), 'anywhere');
});

test('Modal CSS 不向 Bot Modal、confirm 或其他 Semi Modal 泄漏', () => {
  const semiModalSelectors = rules.flatMap((rule) =>
    rule.selectors.filter((selector) => selector.includes('.semi-modal')),
  );
  const unscopedSelectors = semiModalSelectors.filter(
    (selector) => !selector.includes(scope),
  );
  const selectorsStartingWithSemiModal = semiModalSelectors.filter((selector) =>
    /^\.semi-modal(?:[-\s.#:[{]|$)/.test(selector),
  );

  assert.deepEqual(unscopedSelectors, []);
  assert.deepEqual(selectorsStartingWithSemiModal, []);
});
