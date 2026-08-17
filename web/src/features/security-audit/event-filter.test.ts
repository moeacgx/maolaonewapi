/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.
*/
import { describe, expect, it } from 'vitest'

import {
  cleanSecurityAuditEventFilter,
  hasSecurityAuditEventFilter,
} from './event-filter'

describe('security audit event filters', () => {
  it('keeps valid channel filters and discards unset numeric filters', () => {
    expect(cleanSecurityAuditEventFilter({ channel_id: 42 })).toEqual({
      channel_id: 42,
    })
    expect(
      cleanSecurityAuditEventFilter({ channel_id: 0, token_id: -1 })
    ).toEqual({})
  })

  it('normalizes username and decision text before requests or deletion previews', () => {
    expect(
      cleanSecurityAuditEventFilter({
        username: '  audit-reviewer  ',
        decision: ' flag ',
        token_id: 0,
      })
    ).toEqual({
      username: 'audit-reviewer',
      decision: 'flag',
    })
    expect(cleanSecurityAuditEventFilter({ username: '   ' })).toEqual({})
  })

  it('reports whether a normalized filter is active', () => {
    expect(
      hasSecurityAuditEventFilter({ username: '   ', channel_id: 0 })
    ).toBe(false)
    expect(hasSecurityAuditEventFilter({ username: ' reviewer ' })).toBe(true)
  })
})
