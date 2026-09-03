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
