import assert from 'node:assert/strict'
import { test } from 'node:test'
const entryUrl = new URL(
  '../../../extension/builtin/conversation-archive/public/native/classic.mjs',
  import.meta.url
).href

test('conversation archive Classic entry accepts the Classic host SDK', async () => {
  const originalSdk = globalThis.__NEW_API_EXTENSION_NATIVE_SDK__
  globalThis.__NEW_API_EXTENSION_NATIVE_SDK__ = {
    platform: 'classic',
    sdk: 'v1',
    modules: {
      react: {
        default: {},
        Fragment: Symbol('Fragment'),
        useEffect: () => undefined,
        useMemo: () => undefined,
        useState: () => undefined,
      },
      'react/jsx-runtime': {
        Fragment: Symbol('Fragment'),
        jsx: () => null,
        jsxs: () => null,
      },
      'react-i18next': {
        useTranslation: () => ({ t: (key) => key }),
      },
      '../../helpers': {
        API: { get: async () => ({ data: { success: true, data: {} } }) },
      },
    },
  }

  try {
    const entry = await import(`${entryUrl}?test=${Date.now()}`)
    assert.equal(typeof entry.default, 'function')
  } finally {
    if (originalSdk === undefined) {
      delete globalThis.__NEW_API_EXTENSION_NATIVE_SDK__
    } else {
      globalThis.__NEW_API_EXTENSION_NATIVE_SDK__ = originalSdk
    }
  }
})

test('conversation archive Classic entry keeps non-critical group failures isolated', async () => {
  const originalSdk = globalThis.__NEW_API_EXTENSION_NATIVE_SDK__
  const calls = []
  globalThis.__NEW_API_EXTENSION_NATIVE_SDK__ = {
    platform: 'classic',
    sdk: 'v1',
    modules: {
      react: {
        default: {},
        Fragment: Symbol('Fragment'),
        useEffect: () => undefined,
        useState: () => undefined,
      },
      'react/jsx-runtime': {
        Fragment: Symbol('Fragment'),
        jsx: () => null,
        jsxs: () => null,
      },
      'react-i18next': {
        useTranslation: () => ({ t: (key) => key }),
      },
      '../../helpers': {
        getAPI: () => ({
          get: async (url) => {
            calls.push(url)
            if (url.endsWith('/groups')) throw new Error('groups unavailable')
            return { data: { success: true, data: { config_version: 1 } } }
          },
        }),
      },
    },
  }

  try {
    const entry = await import(`${entryUrl}?isolated=${Date.now()}`)
    const result = await entry.loadConversationArchiveData()
    assert.deepEqual(result.config, { config_version: 1 })
    const groups = await result.groups
    assert.deepEqual(groups.value, [])
    assert.equal(groups.error?.message, 'groups unavailable')
    assert.deepEqual(calls, [
      '/api/extensions/conversation-archive/config',
      '/api/extensions/conversation-archive/groups',
    ])
  } finally {
    if (originalSdk === undefined) {
      delete globalThis.__NEW_API_EXTENSION_NATIVE_SDK__
    } else {
      globalThis.__NEW_API_EXTENSION_NATIVE_SDK__ = originalSdk
    }
  }
})
