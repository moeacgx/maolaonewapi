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
import { describe, expect, it } from 'vitest'

import {
  buildHighlightedTextSegments,
  normalizeMatchedKeywords,
} from './matched-keyword-highlight'

describe('security audit matched keyword highlighting', () => {
  it('matches case-insensitively and treats regular expression characters literally', () => {
    const segments = buildHighlightedTextSegments('Alpha a+b ALPHA [x]', [
      'alpha',
      'A+B',
      '[x]',
    ])

    expect(
      segments
        .filter((segment) => segment.highlighted)
        .map((segment) => segment.text)
    ).toEqual(['Alpha', 'a+b', 'ALPHA', '[x]'])
    expect(segments.map((segment) => segment.text).join('')).toBe(
      'Alpha a+b ALPHA [x]'
    )
  })

  it('merges overlapping keyword matches without losing source text', () => {
    const segments = buildHighlightedTextSegments('foo-bar-baz', [
      'foo-bar',
      'bar-baz',
      'bar',
    ])

    expect(segments).toEqual([{ text: 'foo-bar-baz', highlighted: true }])
  })

  it('ignores empty and case-insensitive duplicate keywords', () => {
    expect(normalizeMatchedKeywords(['', '  ', 'Blocked', 'blocked'])).toEqual([
      'Blocked',
    ])
    expect(buildHighlightedTextSegments('unchanged', [])).toEqual([
      { text: 'unchanged', highlighted: false },
    ])
  })

  it('keeps Unicode code points intact around a match', () => {
    expect(buildHighlightedTextSegments('前😀触发词后', ['触发词'])).toEqual([
      { text: '前😀', highlighted: false },
      { text: '触发词', highlighted: true },
      { text: '后', highlighted: false },
    ])
  })

  it('uses the same smart boundary as sensitive keyword matching', () => {
    const segments = buildHighlightedTextSegments(
      'Webmaster Keyword / Master Key / Master Keywordization / 包含敏感词内容',
      ['Master Key', '敏感词']
    )

    expect(
      segments
        .filter((segment) => segment.highlighted)
        .map((segment) => segment.text)
    ).toEqual(['Master Key', '敏感词'])
  })
})
