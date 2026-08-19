import { describe, expect, test } from 'vitest'

import { resolveSelectAllSelection } from './multi-select-selection'

describe('resolveSelectAllSelection', () => {
  test('selects each available real value once', () => {
    expect(resolveSelectAllSelection(['legacy'], ['2', '1', '2', ''])).toEqual([
      '2',
      '1'
    ])
  })

  test('clears the selection when all available values are selected', () => {
    expect(resolveSelectAllSelection(['1', '2'], ['2', '1'])).toEqual([])
  })

  test('keeps the selection when no values are available', () => {
    expect(resolveSelectAllSelection(['legacy'], [])).toEqual(['legacy'])
  })
})
