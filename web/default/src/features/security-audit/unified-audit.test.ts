/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program. If not, see <https://www.gnu.org/licenses/>.

For commercial licensing, please contact support@quantumnous.com
*/
import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import { dirname, resolve } from 'node:path'
import { describe, test } from 'node:test'
import { fileURLToPath } from 'node:url'

const root = dirname(fileURLToPath(import.meta.url))
const readSource = (...parts: string[]) =>
  readFileSync(resolve(root, ...parts), 'utf8')

describe('unified security audit management page', () => {
  test('uses the dedicated Root built-in policy API', () => {
    const api = readSource('api.ts')
    const view = readSource('builtin-policy-view.tsx')

    assert.match(api, /getSecurityAuditBuiltinPolicy/)
    assert.match(api, /updateSecurityAuditBuiltinPolicy/)
    assert.match(api, /\$\{API_ROOT\}\/builtin-policy/)
    assert.match(view, /expected_version:\s*draft\.config_version/)
  })

  test('keeps built-in policy as a first-class audit tab', () => {
    const page = readSource('index.tsx')
    const systemRegistry = readSource(
      '..',
      'system-settings',
      'security',
      'section-registry.tsx'
    )

    assert.match(page, /value:\s*'builtin-policy'/)
    assert.match(page, /SecurityAuditBuiltinPolicyView/)
    assert.doesNotMatch(systemRegistry, /id:\s*'sensitive-words'/)
  })

  test('shows event source, stage, and unavailable prompt state', () => {
    const events = readSource('events-view.tsx')

    assert.match(events, /draftFilter\.source/)
    assert.match(events, /draftFilter\.stage/)
    assert.match(events, /detail\.prompt_available/)
    assert.match(events, /Prompt content was not stored/)
  })

  test('saves migrated sensitive-word rules atomically', () => {
    const view = readSource('builtin-policy-view.tsx')
    const editor = readSource(
      '..',
      'system-settings',
      'request-limits',
      'sensitive-words-section.tsx'
    )

    assert.match(view, /sensitive_rules:\s*values\.SensitiveRules/)
    assert.match(view, /sensitive_rule_channel_ids:/)
    assert.match(editor, /onSaveValues/)
    assert.match(editor, /inlineActions/)
    assert.doesNotMatch(editor, /\}, \[defaultValues\]\)/)
  })

  test('keeps complete request archiving on an independent write-only contract', () => {
    const api = readSource('api.ts')
    const page = readSource('index.tsx')
    const view = readSource('request-archive-view.tsx')
    const types = readSource('types.ts')

    assert.match(api, /request-archive\/config/)
    assert.match(api, /request-archive\/runtime/)
    assert.match(api, /request-archive\/targets\/probe/)
    assert.match(page, /value:\s*'request-archive'/)
    assert.match(view, /type='password'/)
    assert.match(types, /max_body_bytes:\s*number/)
    assert.match(types, /queue_max_bytes:\s*number/)
    assert.match(types, /access_key_configured:\s*boolean/)
    assert.match(types, /secret_key_configured:\s*boolean/)
    assert.match(types, /RequestArchiveApiErrorResponse/)
  })

  test('preserves request archive drafts for non-CAS conflicts', () => {
    const view = readSource('request-archive-view.tsx')

    assert.match(
      view,
      /error\.response\.data\?\.code === 'request_archive_config_conflict'/
    )
    assert.doesNotMatch(
      view,
      /axios\.isAxiosError\(error\) && error\.response\?\.status === 409/
    )
    assert.match(view, /getErrorMessage\(\s*error,/)
  })

  test('does not render a failed request archive runtime as stopped', () => {
    const view = readSource('request-archive-view.tsx')

    assert.match(view, /if \(error && !runtime\)/)
    assert.match(view, /Request archive runtime is unavailable/)
    assert.match(
      view,
      /Showing the last known runtime because the latest refresh failed\./
    )
    assert.match(
      view,
      /error=\{runtimeQuery\.isError \? runtimeQuery\.error : null\}/
    )
    assert.match(view, /onRetry=\{\(\) => void runtimeQuery\.refetch\(\)\}/)
  })
})
