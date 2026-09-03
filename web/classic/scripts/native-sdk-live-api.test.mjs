import assert from 'node:assert/strict'
import { test } from 'node:test'

test('Classic native SDK exposes the current API instance after auth refresh', async () => {
  const { createClassicNativeHelpers } = await import(
    '../src/pages/Extensions/native-sdk-helpers.js',
  )
  let currentApi = { token: 'before-refresh' }
  const helpers = createClassicNativeHelpers(() => currentApi)

  assert.equal(helpers.API, currentApi)
  assert.equal(helpers.getAPI(), currentApi)
  currentApi = { token: 'after-refresh' }
  assert.equal(helpers.API, currentApi)
  assert.equal(helpers.getAPI(), currentApi)
  assert.equal(Object.isFrozen(helpers), true)
})
