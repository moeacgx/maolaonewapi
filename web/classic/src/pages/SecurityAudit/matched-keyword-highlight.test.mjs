import assert from 'node:assert/strict';
import { describe, test } from 'node:test';
import {
  buildHighlightedTextSegments,
  createKeywordHighlightPlugin,
  normalizeMatchedKeywords,
} from './matched-keyword-highlight.js';

describe('Classic 安全审计命中词高亮', () => {
  test('大小写不敏感且按字面匹配特殊字符', () => {
    const segments = buildHighlightedTextSegments('Alpha a+b ALPHA [x]', [
      'alpha',
      'A+B',
      '[x]',
    ]);

    assert.deepEqual(
      segments
        .filter((segment) => segment.highlighted)
        .map((segment) => segment.text),
      ['Alpha', 'a+b', 'ALPHA', '[x]'],
    );
    assert.equal(
      segments.map((segment) => segment.text).join(''),
      'Alpha a+b ALPHA [x]',
    );
  });

  test('合并重叠命中范围且不丢失原文', () => {
    assert.deepEqual(
      buildHighlightedTextSegments('foo-bar-baz', [
        'foo-bar',
        'bar-baz',
        'bar',
      ]),
      [{ text: 'foo-bar-baz', highlighted: true }],
    );
  });

  test('忽略空关键词和大小写重复关键词', () => {
    assert.deepEqual(
      normalizeMatchedKeywords(['', '  ', 'Blocked', 'blocked']),
      ['Blocked'],
    );
    assert.deepEqual(buildHighlightedTextSegments('unchanged', []), [
      { text: 'unchanged', highlighted: false },
    ]);
  });

  test('按 Unicode 码点切分，不破坏表情符号', () => {
    assert.deepEqual(buildHighlightedTextSegments('前😀触发词后', ['触发词']), [
      { text: '前😀', highlighted: false },
      { text: '触发词', highlighted: true },
      { text: '后', highlighted: false },
    ]);
  });

  test('与屏蔽词运行时使用相同的智能边界', () => {
    const segments = buildHighlightedTextSegments(
      'Webmaster Keyword / Master Key / Master Keywordization / 包含敏感词内容',
      ['Master Key', '敏感词'],
    );

    assert.deepEqual(
      segments
        .filter((segment) => segment.highlighted)
        .map((segment) => segment.text),
      ['Master Key', '敏感词'],
    );
  });

  test('Markdown 文本节点高亮时保留原有元素层级', () => {
    const tree = {
      type: 'root',
      children: [
        {
          type: 'element',
          tagName: 'p',
          properties: {},
          children: [{ type: 'text', value: '前文 Alpha 后文' }],
        },
      ],
    };
    const transform = createKeywordHighlightPlugin(['alpha'])();
    transform(tree);

    assert.equal(tree.children[0].tagName, 'p');
    assert.equal(tree.children[0].children[1].tagName, 'mark');
    assert.equal(tree.children[0].children[1].children[0].value, 'Alpha');
  });
});
