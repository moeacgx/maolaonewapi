/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.
*/
import { afterEach, describe, expect, test, vi } from 'vitest'

import { resolvePreviewImage } from './image-preview'

afterEach(() => {
  vi.restoreAllMocks()
  vi.unstubAllGlobals()
})

describe('task image preview lifecycle', () => {
  test('leaves public URLs untouched without fetching a blob', async () => {
    const loadBlob = vi.fn()
    const result = await resolvePreviewImage(
      'https://example.com/result.png',
      new AbortController().signal,
      loadBlob
    )

    expect(result.src).toBe('https://example.com/result.png')
    expect(loadBlob).not.toHaveBeenCalled()
  })

  test('creates and revokes one authenticated Blob URL', async () => {
    const create = vi.fn().mockReturnValue('blob:task-result')
    const revoke = vi.fn()
    vi.stubGlobal('URL', {
      createObjectURL: create,
      revokeObjectURL: revoke,
    })
    const controller = new AbortController()
    const result = await resolvePreviewImage(
      '/api/task/t1/image',
      controller.signal,
      vi.fn().mockResolvedValue(new Blob(['image']))
    )

    expect(create).toHaveBeenCalledOnce()
    expect(result.src).toBe('blob:task-result')
    result.release()
    result.release()
    expect(revoke).toHaveBeenCalledOnce()
    expect(revoke).toHaveBeenCalledWith('blob:task-result')
  })
})
