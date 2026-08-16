/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.
*/
import { describe, expect, test } from 'vitest'

import { getTaskLogModelDisplay, getTaskResultKind } from './task-log-format'

describe('task log transformations', () => {
  test('uses the requested model and exposes a distinct actual model', () => {
    expect(
      getTaskLogModelDisplay({
        properties: {
          origin_model_name: 'gpt-image-1',
          upstream_model_name: 'openai/gpt-image-1',
        },
      })
    ).toEqual({
      requestModel: 'gpt-image-1',
      actualModel: 'openai/gpt-image-1',
    })
  })

  test('falls back to the upstream model for legacy records', () => {
    expect(
      getTaskLogModelDisplay({
        properties: { upstream_model_name: 'sora-2' },
      })
    ).toEqual({ requestModel: 'sora-2', actualModel: undefined })
  })

  test('gives deliverable images precedence over expiry and detail text', () => {
    expect(
      getTaskResultKind({
        status: 'SUCCESS',
        image_urls: ['/api/task/t1/image'],
        result_expired: true,
        fail_reason: 'https://example.com/video',
      })
    ).toBe('images')
  })
})
