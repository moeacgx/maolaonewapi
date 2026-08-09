import assert from 'node:assert/strict'
import { dirname, resolve } from 'node:path'
import { readFileSync } from 'node:fs'
import { test } from 'node:test'
import { fileURLToPath } from 'node:url'

const root = dirname(fileURLToPath(import.meta.url))
const readSource = (file: string) => readFileSync(resolve(root, file), 'utf8')

test('unlimited API keys still expose used quota in desktop and mobile cells', () => {
  const cellsSource = readSource('components/api-keys-cells.tsx')
  const columnsSource = readSource('components/api-keys-columns.tsx')
  const tableSource = readSource('components/api-keys-table.tsx')

  assert.match(cellsSource, /export function UnlimitedQuotaBadge/)
  assert.match(cellsSource, /formatQuota\(props\.used\)/)
  assert.match(cellsSource, /aria-label=\{`\$\{t\('Unlimited'\)\}; \$\{t\('Used:'\)\}/)
  assert.match(columnsSource, /<UnlimitedQuotaBadge used=\{apiKey\.used_quota\} \/>/)
  assert.match(tableSource, /<UnlimitedQuotaBadge used=\{apiKey\.used_quota\} \/>/)
})
