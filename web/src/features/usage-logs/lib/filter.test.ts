/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.
*/
import { describe, expect, test } from 'vitest'

import { buildSearchParams } from './filter'

describe('usage log filter params', () => {
  test('task logs preserve model and admin username filters', () => {
    const params = buildSearchParams(
      {
        taskId: 'task-123',
        model: 'gpt-image-1',
        username: 'alice',
        channel: '7',
      },
      'task'
    )

    expect(params).toMatchObject({
      filter: 'task-123',
      model: 'gpt-image-1',
      username: 'alice',
      channel: '7',
    })
  })
})
