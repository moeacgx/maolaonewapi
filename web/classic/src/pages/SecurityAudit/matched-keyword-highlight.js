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

export function normalizeMatchedKeywords(keywords) {
  const result = [];
  const seen = new Set();
  for (const rawKeyword of keywords || []) {
    const keyword = String(rawKeyword || '').trim();
    if (!keyword) continue;
    const folded = Array.from(keyword, (character) =>
      character.toLowerCase(),
    ).join('');
    if (seen.has(folded)) continue;
    seen.add(folded);
    result.push(keyword);
  }
  return result;
}

function isASCIISensitiveWordCharacter(value) {
  if (!value || value.length !== 1) return false;
  const code = value.charCodeAt(0);
  return (
    (code >= 48 && code <= 57) ||
    (code >= 65 && code <= 90) ||
    (code >= 97 && code <= 122) ||
    code === 95
  );
}

function hasSensitiveKeywordBoundary(source, keyword, start) {
  const end = start + keyword.length;
  const startBoundary =
    !isASCIISensitiveWordCharacter(keyword[0]) ||
    start === 0 ||
    !isASCIISensitiveWordCharacter(source[start - 1]);
  const endBoundary =
    !isASCIISensitiveWordCharacter(keyword[keyword.length - 1]) ||
    end === source.length ||
    !isASCIISensitiveWordCharacter(source[end]);
  return startBoundary && endBoundary;
}

export function buildHighlightedTextSegments(text, keywords) {
  if (!text) return [];
  const source = Array.from(text);
  const foldedSource = source.map((character) => character.toLowerCase());
  const ranges = [];
  for (const keyword of normalizeMatchedKeywords(keywords)) {
    const foldedKeyword = Array.from(keyword, (character) =>
      character.toLowerCase(),
    );
    for (
      let start = 0;
      start <= source.length - foldedKeyword.length;
      start += 1
    ) {
      let matched = true;
      for (let offset = 0; offset < foldedKeyword.length; offset += 1) {
        if (foldedSource[start + offset] !== foldedKeyword[offset]) {
          matched = false;
          break;
        }
      }
      if (
        matched &&
        hasSensitiveKeywordBoundary(foldedSource, foldedKeyword, start)
      ) {
        ranges.push({ start, end: start + foldedKeyword.length });
      }
    }
  }
  ranges.sort(
    (left, right) => left.start - right.start || right.end - left.end,
  );
  const merged = [];
  for (const range of ranges) {
    const previous = merged[merged.length - 1];
    if (previous && range.start <= previous.end) {
      previous.end = Math.max(previous.end, range.end);
    } else {
      merged.push({ ...range });
    }
  }
  if (merged.length === 0) return [{ text, highlighted: false }];
  const result = [];
  let cursor = 0;
  for (const range of merged) {
    if (range.start > cursor) {
      result.push({
        text: source.slice(cursor, range.start).join(''),
        highlighted: false,
      });
    }
    result.push({
      text: source.slice(range.start, range.end).join(''),
      highlighted: true,
    });
    cursor = range.end;
  }
  if (cursor < source.length) {
    result.push({ text: source.slice(cursor).join(''), highlighted: false });
  }
  return result;
}

// 在 Markdown 解析后的文本节点中包裹 mark，保留原有 Markdown 渲染能力。
export function createKeywordHighlightPlugin(keywords) {
  const normalized = normalizeMatchedKeywords(keywords);
  return () => (tree) => {
    const walk = (node) => {
      if (!node || !Array.isArray(node.children)) return;
      const nextChildren = [];
      for (const child of node.children) {
        if (child.type === 'text' && normalized.length > 0) {
          for (const segment of buildHighlightedTextSegments(
            child.value,
            normalized,
          )) {
            if (segment.highlighted) {
              nextChildren.push({
                type: 'element',
                tagName: 'mark',
                properties: {
                  className:
                    'rounded-sm bg-red-100 px-0 text-red-700 dark:bg-red-950/70 dark:text-red-300',
                },
                children: [{ type: 'text', value: segment.text }],
              });
            } else {
              nextChildren.push({ type: 'text', value: segment.text });
            }
          }
        } else {
          walk(child);
          nextChildren.push(child);
        }
      }
      node.children = nextChildren;
    };
    walk(tree);
  };
}
