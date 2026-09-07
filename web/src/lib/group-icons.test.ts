import { describe, expect, test } from 'vitest'

import { filterGroupIconOptions } from './group-icons'

describe('group icon catalog', () => {
  test('filters AI provider icons by key or label', () => {
    expect(filterGroupIconOptions('deep')).toEqual([
      expect.objectContaining({ value: 'DeepSeek.Color' }),
    ])
    expect(filterGroupIconOptions('谷歌')).toEqual([
      expect.objectContaining({ value: 'Gemini.Color' }),
    ])
  })

  test('returns the full curated catalog for an empty query', () => {
    expect(filterGroupIconOptions('')).toHaveLength(35)
  })
})
