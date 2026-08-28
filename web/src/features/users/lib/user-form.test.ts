/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as published by
the Free Software Foundation, either version 3 of the License, or
(at your option) any later version.
*/
import { describe, expect, test } from 'vitest'

import { DEFAULT_GROUP } from '../constants'
import { transformFormDataToPayload, USER_FORM_DEFAULT_VALUES } from './user-form'

describe('user form API payload contracts', () => {
  test('includes the default group when creating a user', () => {
    const payload = transformFormDataToPayload({
      ...USER_FORM_DEFAULT_VALUES,
      username: 'new-user',
      password: 'password',
      group: '',
    })

    expect(payload.group).toBe(DEFAULT_GROUP)
  })

  test('preserves an explicitly selected group when creating a user', () => {
    const payload = transformFormDataToPayload({
      ...USER_FORM_DEFAULT_VALUES,
      username: 'vip-user',
      password: 'password',
      group: 'vip',
    })

    expect(payload.group).toBe('vip')
  })
})
