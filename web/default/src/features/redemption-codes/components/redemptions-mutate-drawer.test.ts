import assert from 'node:assert/strict'
import { dirname, resolve } from 'node:path'
import { readFileSync } from 'node:fs'
import { test } from 'node:test'
import { fileURLToPath } from 'node:url'

const root = dirname(fileURLToPath(import.meta.url))
const readSource = (file: string) => readFileSync(resolve(root, file), 'utf8')
const readLibSource = (file: string) =>
  readFileSync(resolve(root, '../lib', file), 'utf8')

test('redemption update waits for a fresh load and ignores stale responses', () => {
  const drawerSource = readSource('redemptions-mutate-drawer.tsx')

  assert.match(drawerSource, /redemptionLoadState/)
  assert.match(drawerSource, /loadedRedemption\?\.id === redemptionId/)
  assert.match(drawerSource, /let ignoreResult = false/)
  assert.match(drawerSource, /if \(ignoreResult\) return/)
  assert.match(drawerSource, /disabled=\{isSubmitting \|\| !isUpdateReady\}/)
})

test('redemption update preserves original quota unless quota field is dirty', () => {
  const drawerSource = readSource('redemptions-mutate-drawer.tsx')

  assert.match(drawerSource, /form\.getFieldState\('quota_dollars'\)\.isDirty/)
  assert.match(drawerSource, /: loadedRedemption\.quota/)
})

test('redemption editable quota uses rounded display precision', () => {
  const drawerSource = readSource('redemptions-mutate-drawer.tsx')
  const formSource = readLibSource('redemption-form.ts')

  assert.match(drawerSource, /getEditableQuotaStep\(\)/)
  assert.match(formSource, /quotaUnitsToEditableAmount\(redemption\.quota\)/)
  assert.doesNotMatch(formSource, /quotaUnitsToDollars\(redemption\.quota\)/)
})
