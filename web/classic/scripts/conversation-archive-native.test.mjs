import assert from 'node:assert/strict'
import { test } from 'node:test'
const entryUrl = new URL(
  '../../../extension/builtin/conversation-archive/public/native/classic.mjs',
  import.meta.url
).href
const entrySource = await import('node:fs/promises').then(({ readFile }) =>
  readFile(new URL(entryUrl), 'utf8'),
)
const stylesheetSource = await import('node:fs/promises').then(({ readFile }) =>
  readFile(new URL('../../../extension/builtin/conversation-archive/public/native/classic.css', import.meta.url), 'utf8'),
)

function getRuleBodies(selector) {
  const escapedSelector = selector.replace(/[.*+?^${}()|[\]\\]/g, '\\$&')
  return [...stylesheetSource.matchAll(new RegExp(`${escapedSelector}\\s*\\{([^}]*)\\}`, 'g'))]
    .map((match) => match[1])
}

test('conversation archive Classic page uses one visible business shell', () => {
  assert.match(entrySource, /className: 'conversation-archive-native'/)
  assert.match(entrySource, /className: 'archive-page-shell'/)
  assert.match(entrySource, /className: 'archive-page-header'/)
  assert.match(entrySource, /className: 'archive-page-content'/)
  assert.match(stylesheetSource, /\.archive-page-shell\s*\{[\s\S]*border:[^;]+;/)
  assert.match(stylesheetSource, /\.archive-page-shell\s*\{[\s\S]*border-radius:[^;]+;/)
  assert.match(stylesheetSource, /\.archive-page-shell\s*\{[\s\S]*background:[^;]+;/)
})

test('conversation archive Classic preview is a centered modal overlay', () => {
  const backdropRules = getRuleBodies('.archive-modal-backdrop')
  const modalRule = getRuleBodies('.archive-modal').find((rule) => rule.includes('max-height:'))
  const modalContentRule = getRuleBodies('.archive-modal-content').find((rule) => rule.includes('overflow:'))

  assert.match(entrySource, /className: 'archive-modal-backdrop'/)
  assert.match(entrySource, /className: 'archive-modal'/)
  assert.match(entrySource, /role: 'dialog'/)
  assert.match(entrySource, /'aria-modal': true/)
  assert.match(entrySource, /'aria-labelledby':/)
  assert.match(entrySource, /event\.key === 'Escape'/)
  assert.match(entrySource, /event\.target === event\.currentTarget/)
  assert.match(entrySource, /useRef/)
  assert.match(entrySource, /tabIndex: -1/)
  assert.match(entrySource, /querySelectorAll/)
  assert.match(entrySource, /triggerRef\.current\?\.focus/)
  assert.match(entrySource, /body\.style\.overflow = 'hidden'/)
  assert.match(entrySource, /body\.style\.overflow = previousBodyOverflow/)
  assert.doesNotMatch(entrySource, /archive-preview-section/)
  assert.ok(backdropRules.length > 0)
  assert.match(backdropRules[0], /position:\s*fixed;/)
  assert.match(backdropRules[0], /inset:\s*0;/)
  assert.match(backdropRules[0], /align-items:\s*center;/)
  assert.match(backdropRules[0], /justify-content:\s*center;/)
  assert.doesNotMatch(backdropRules.join('\n'), /align-items:\s*flex-end;/)
  assert.match(modalRule, /max-height:/)
  assert.match(modalContentRule, /overflow:\s*auto;/)
})

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
