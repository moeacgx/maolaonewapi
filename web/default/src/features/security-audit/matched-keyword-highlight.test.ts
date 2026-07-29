/*
Copyright (C) 2023-2026 QuantumNous

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
import assert from 'node:assert/strict'
import { describe, test } from 'node:test'
import {
  buildHighlightedTextSegments,
  normalizeMatchedKeywords,
} from './matched-keyword-highlight.ts'

describe('security audit matched keyword highlighting', () => {
  test('matches case-insensitively and treats regular expression characters literally', () => {
    const segments = buildHighlightedTextSegments('Alpha a+b ALPHA [x]', [
      'alpha',
      'A+B',
      '[x]',
    ])

    assert.deepEqual(
      segments
        .filter((segment) => segment.highlighted)
        .map((segment) => segment.text),
      ['Alpha', 'a+b', 'ALPHA', '[x]']
    )
    assert.equal(
      segments.map((segment) => segment.text).join(''),
      'Alpha a+b ALPHA [x]'
    )
  })

  test('merges overlapping keyword matches without losing source text', () => {
    const segments = buildHighlightedTextSegments('foobarbaz', [
      'foobar',
      'barbaz',
      'bar',
    ])

    assert.deepEqual(segments, [{ text: 'foobarbaz', highlighted: true }])
  })

  test('ignores empty and case-insensitive duplicate keywords', () => {
    assert.deepEqual(
      normalizeMatchedKeywords(['', '  ', 'Blocked', 'blocked']),
      ['Blocked']
    )
    assert.deepEqual(buildHighlightedTextSegments('unchanged', []), [
      { text: 'unchanged', highlighted: false },
    ])
  })

  test('keeps Unicode code points intact around a match', () => {
    assert.deepEqual(buildHighlightedTextSegments('前😀触发词后', ['触发词']), [
      { text: '前😀', highlighted: false },
      { text: '触发词', highlighted: true },
      { text: '后', highlighted: false },
    ])
  })
})
