import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import { dirname, resolve } from 'node:path'
import { test } from 'node:test'
import { fileURLToPath } from 'node:url'

const root = dirname(fileURLToPath(import.meta.url))
const readSource = (file: string) => readFileSync(resolve(root, file), 'utf8')

test('model drawer load effect does not depend on refetched model settings object identity', () => {
  const source = readSource('model-mutate-drawer.tsx')
  const loadEffect = source.match(
    /\/\/ Load model data for editing and ratio configuration[\s\S]*?\n {2}\}, \[[^\n]+\]\)/
  )?.[0]

  assert.ok(loadEffect)
  assert.match(
    source,
    /modelSettingsRef = useRef<ModelSettings \| null>\(null\)/
  )
  assert.match(source, /modelSettingsRef\.current = modelSettings/)
  assert.match(loadEffect, /modelSettingsRef\.current/)
  assert.match(loadEffect, /hasModelSettings/)
  assert.doesNotMatch(loadEffect, /, modelSettings\]/)
})
