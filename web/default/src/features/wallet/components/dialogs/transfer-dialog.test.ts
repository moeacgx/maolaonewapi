import assert from 'node:assert/strict'
import { dirname, resolve } from 'node:path'
import { readFileSync } from 'node:fs'
import { test } from 'node:test'
import { fileURLToPath } from 'node:url'

const root = dirname(fileURLToPath(import.meta.url))
const readSource = (file: string) => readFileSync(resolve(root, file), 'utf8')

test('wallet reward transfer uses configured quota units instead of hard-coded dollars', () => {
  const dialogSource = readSource('transfer-dialog.tsx')
  const constantsSource = readFileSync(resolve(root, '../../constants.ts'), 'utf8')

  assert.match(dialogSource, /currencyConfig\.quotaPerUnit/)
  assert.match(dialogSource, /parseQuotaFromDollars\(amount\)/)
  assert.match(dialogSource, /onConfirm\(transferQuota\)/)
  assert.match(dialogSource, /transferQuota >= minimumQuota/)
  assert.doesNotMatch(dialogSource, /QUOTA_PER_DOLLAR/)
  assert.doesNotMatch(constantsSource, /QUOTA_PER_DOLLAR/)
})
