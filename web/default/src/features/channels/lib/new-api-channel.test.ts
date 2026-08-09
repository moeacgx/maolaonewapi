import assert from 'node:assert/strict'
import { describe, test } from 'node:test'

import {
  CHANNEL_TYPE_ADVANCED_CUSTOM,
  CHANNEL_TYPE_NEW_API,
  CHANNEL_TYPE_OPTIONS,
  CHANNEL_TYPE_SUB2API,
  CHANNEL_TYPES,
  MODEL_FETCHABLE_TYPES,
  TYPE_TO_KEY_PROMPT,
} from '../constants.ts'
import { getModelCategory } from './model-categories.ts'

describe('New API channel frontend metadata', () => {
  test('registers official relay channel types as model-fetchable options', () => {
    assert.equal(CHANNEL_TYPE_ADVANCED_CUSTOM, 58)
    assert.equal(CHANNEL_TYPE_SUB2API, 59)
    assert.equal(CHANNEL_TYPE_NEW_API, 60)

    assert.equal(CHANNEL_TYPES[CHANNEL_TYPE_ADVANCED_CUSTOM], 'Advanced Custom')
    assert.equal(CHANNEL_TYPES[CHANNEL_TYPE_SUB2API], 'Sub2API')
    assert.equal(CHANNEL_TYPES[CHANNEL_TYPE_NEW_API], 'New API')

    for (const type of [
      CHANNEL_TYPE_ADVANCED_CUSTOM,
      CHANNEL_TYPE_SUB2API,
      CHANNEL_TYPE_NEW_API,
    ]) {
      assert.equal(MODEL_FETCHABLE_TYPES.has(type), true)
    }

    assert.equal(
      TYPE_TO_KEY_PROMPT[CHANNEL_TYPE_SUB2API],
      'Enter API key for this channel'
    )

    const values = CHANNEL_TYPE_OPTIONS.map((option) => option.value)
    assert.deepEqual(
      values.slice(
        values.indexOf(CHANNEL_TYPE_NEW_API),
        values.indexOf(CHANNEL_TYPE_NEW_API) + 2
      ),
      [CHANNEL_TYPE_NEW_API, CHANNEL_TYPE_ADVANCED_CUSTOM]
    )
    assert.deepEqual(
      values.slice(values.indexOf(57), values.indexOf(57) + 2),
      [57, CHANNEL_TYPE_SUB2API]
    )

    assert.equal(
      CHANNEL_TYPE_OPTIONS.find(
        (option) => option.value === CHANNEL_TYPE_ADVANCED_CUSTOM
      )?.label,
      'Advanced Custom'
    )
    assert.equal(
      CHANNEL_TYPE_OPTIONS.find((option) => option.value === CHANNEL_TYPE_SUB2API)
        ?.label,
      'Sub2API'
    )
    assert.equal(
      CHANNEL_TYPE_OPTIONS.find((option) => option.value === CHANNEL_TYPE_NEW_API)
        ?.label,
      'New API'
    )
  })

  test('classifies Qwen TTS models without treating tts prefix as OpenAI', () => {
    assert.equal(getModelCategory('qwen-tts-latest'), 'Qwen')
    assert.equal(getModelCategory('tts-1'), 'OpenAI')
  })
})
