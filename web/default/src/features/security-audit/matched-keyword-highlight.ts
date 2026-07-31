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

export interface HighlightedTextSegment {
  text: string
  highlighted: boolean
}

interface TextRange {
  start: number
  end: number
}

function foldCodePoints(value: string) {
  return Array.from(value, (character) => character.toLowerCase())
}

function isASCIISensitiveWordCharacter(value?: string) {
  if (!value || value.length !== 1) return false
  const code = value.charCodeAt(0)
  return (
    (code >= 48 && code <= 57) ||
    (code >= 65 && code <= 90) ||
    (code >= 97 && code <= 122) ||
    code === 95
  )
}

function hasSensitiveKeywordBoundary(
  source: string[],
  keyword: string[],
  start: number
) {
  const end = start + keyword.length
  const startBoundary =
    !isASCIISensitiveWordCharacter(keyword[0]) ||
    start === 0 ||
    !isASCIISensitiveWordCharacter(source[start - 1])
  const endBoundary =
    !isASCIISensitiveWordCharacter(keyword[keyword.length - 1]) ||
    end === source.length ||
    !isASCIISensitiveWordCharacter(source[end])
  return startBoundary && endBoundary
}

export function normalizeMatchedKeywords(keywords?: readonly string[]) {
  const normalized: string[] = []
  const seen = new Set<string>()

  for (const rawKeyword of keywords ?? []) {
    const keyword = String(rawKeyword ?? '').trim()
    if (!keyword) continue

    const folded = foldCodePoints(keyword).join('')
    if (seen.has(folded)) continue
    seen.add(folded)
    normalized.push(keyword)
  }

  return normalized
}

export function buildHighlightedTextSegments(
  text: string,
  keywords?: readonly string[]
): HighlightedTextSegment[] {
  if (!text) return []

  const source = Array.from(text)
  const foldedSource = foldCodePoints(text)
  const ranges: TextRange[] = []

  for (const keyword of normalizeMatchedKeywords(keywords)) {
    const foldedKeyword = foldCodePoints(keyword)
    if (foldedKeyword.length === 0 || foldedKeyword.length > source.length) {
      continue
    }

    for (
      let start = 0;
      start <= source.length - foldedKeyword.length;
      start++
    ) {
      let matches = true
      for (let offset = 0; offset < foldedKeyword.length; offset++) {
        if (foldedSource[start + offset] !== foldedKeyword[offset]) {
          matches = false
          break
        }
      }
      if (
        matches &&
        hasSensitiveKeywordBoundary(foldedSource, foldedKeyword, start)
      ) {
        ranges.push({ start, end: start + foldedKeyword.length })
      }
    }
  }

  if (ranges.length === 0) {
    return [{ text, highlighted: false }]
  }

  ranges.sort((left, right) => left.start - right.start || right.end - left.end)
  const mergedRanges: TextRange[] = []
  for (const range of ranges) {
    const previous = mergedRanges[mergedRanges.length - 1]
    if (previous && range.start <= previous.end) {
      previous.end = Math.max(previous.end, range.end)
      continue
    }
    mergedRanges.push({ ...range })
  }

  const segments: HighlightedTextSegment[] = []
  let cursor = 0
  for (const range of mergedRanges) {
    if (range.start > cursor) {
      segments.push({
        text: source.slice(cursor, range.start).join(''),
        highlighted: false,
      })
    }
    segments.push({
      text: source.slice(range.start, range.end).join(''),
      highlighted: true,
    })
    cursor = range.end
  }
  if (cursor < source.length) {
    segments.push({
      text: source.slice(cursor).join(''),
      highlighted: false,
    })
  }

  return segments
}

interface HighlightTreeNode {
  type?: string
  value?: string
  tagName?: string
  properties?: Record<string, unknown>
  children?: HighlightTreeNode[]
}

// 在 Markdown 解析后的文本节点中包裹 mark，保留原有 Markdown 渲染能力。
export function createKeywordHighlightPlugin(keywords: readonly string[]) {
  const normalized = normalizeMatchedKeywords(keywords)
  return () => (tree: HighlightTreeNode) => {
    const walk = (node: HighlightTreeNode): void => {
      if (!node.children) return
      const nextChildren: HighlightTreeNode[] = []
      for (const child of node.children) {
        if (child.type === 'text' && normalized.length > 0) {
          for (const segment of buildHighlightedTextSegments(
            child.value ?? '',
            normalized
          )) {
            if (segment.highlighted) {
              nextChildren.push({
                type: 'element',
                tagName: 'mark',
                properties: { 'data-audit-keyword-highlight': true },
                children: [{ type: 'text', value: segment.text }],
              })
            } else {
              nextChildren.push({ type: 'text', value: segment.text })
            }
          }
        } else {
          walk(child)
          nextChildren.push(child)
        }
      }
      node.children = nextChildren
    }
    walk(tree)
  }
}
