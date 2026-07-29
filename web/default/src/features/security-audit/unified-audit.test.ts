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
const readClassicSource = (...parts: string[]) =>
  readFileSync(
    resolve(root, '..', '..', '..', '..', 'classic', 'src', ...parts),
    'utf8'
  )

describe('unified security audit management page', () => {
  test('does not require step-up verification anywhere in security audit', () => {
    const defaultSources = [
      readSource('index.tsx'),
      readSource('endpoints-view.tsx'),
      readSource('events-view.tsx'),
    ].join('\n')
    const classicSources = [
      readClassicSource('pages', 'SecurityAudit', 'index.jsx'),
      readClassicSource('pages', 'SecurityAudit', 'EndpointsTab.jsx'),
      readClassicSource('pages', 'SecurityAudit', 'EventsTab.jsx'),
    ].join('\n')

    for (const source of [defaultSources, classicSources]) {
      assert.doesNotMatch(source, /useSecureVerification/)
      assert.doesNotMatch(source, /SecureVerification(?:Dialog|Modal)/)
      assert.doesNotMatch(source, /runSensitive/)
      assert.doesNotMatch(source, /withVerification/)
    }
  })

  test('uses the dedicated Root built-in policy API', () => {
    const api = readSource('api.ts')
    const view = readSource('builtin-policy-view.tsx')
    const types = readSource('types.ts')

    assert.match(api, /getSecurityAuditBuiltinPolicy/)
    assert.match(api, /updateSecurityAuditBuiltinPolicy/)
    assert.match(api, /\$\{API_ROOT\}\/builtin-policy/)
    assert.match(view, /expected_version:\s*draft\.config_version/)
    assert.match(view, /cyber_policy_auto_ban_enabled/)
    assert.match(view, /cyber_policy_ban_threshold/)
    assert.match(view, /cyber_policy_violation_window_hours/)
    assert.match(
      view,
      /cyberPolicyAutoBanEnabled\s*\|\|\s*current\.upstream_policy_enabled/
    )
    assert.match(
      types,
      /upstream_policy_target_type:\s*UpstreamPolicyTargetType/
    )
    assert.match(types, /upstream_policy_channel_ids:\s*number\[\]/)
    assert.match(types, /upstream_policy_group_codes:\s*string\[\]/)
    assert.match(view, /getSensitiveRuleChannels/)
    assert.match(view, /getSensitiveRuleGroups/)
    assert.match(view, /includeMissingSensitiveRouteOptions/)
    assert.match(view, /includeMissingSensitiveGroupOptions/)
    assert.match(view, /externalInvalid=\{scopeValidationError !== null\}/)
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
    assert.match(events, /Official risk control \(cyber_policy\)/)
    assert.match(events, /draftFilter\.stage/)
    assert.match(events, /detail\.prompt_available/)
    assert.match(events, /This historical event did not retain the prompt body/)
  })

  test('shows each user cyber policy total within the configured window', () => {
    const events = readSource('events-view.tsx')
    const types = readSource('types.ts')
    const classicEvents = readClassicSource(
      'pages',
      'SecurityAudit',
      'EventsTab.jsx'
    )

    assert.match(types, /user_cyber_policy_count:\s*number/)
    assert.match(types, /cyber_policy_window_hours:\s*number/)
    assert.match(events, /t\('Within-window total'\)/)
    assert.match(events, /row\.original\.user_cyber_policy_count/)
    assert.match(events, /row\.original\.cyber_policy_window_hours/)
    assert.match(events, /t\('\{\{count\}\} times', \{ count \}\)/)
    assert.match(events, /t\('Within \{\{hours\}\} hours', \{ hours \}\)/)
    assert.match(classicEvents, /t\('窗口内累计'\)/)
    assert.match(classicEvents, /dataIndex: 'user_cyber_policy_count'/)
    assert.match(classicEvents, /record\.cyber_policy_window_hours/)
    assert.match(classicEvents, /t\('\{\{count\}\} 次', \{ count \}\)/)
    assert.match(classicEvents, /t\('\{\{hours\}\} 小时内', \{ hours \}\)/)
  })

  test('shows recorded channel and channel groups in event lists and details', () => {
    const events = readSource('events-view.tsx')
    const types = readSource('types.ts')
    const routing = readSource('event-routing-display.ts')

    assert.match(types, /channel_id:\s*number/)
    assert.match(types, /channel_name:\s*string/)
    assert.match(types, /channel_groups:\s*SecurityAuditChannelGroup\[\]/)
    assert.match(events, /header:\s*t\('Channel'\)/)
    assert.match(events, /header:\s*t\('Group'\)/)
    assert.match(events, /header:\s*t\('Channel-assigned groups'\)/)
    assert.match(events, /<AuditChannelDisplay event=\{detail\}/)
    assert.match(events, /<AuditRouteGroupDisplay event=\{detail\}/)
    assert.match(events, /<AuditChannelGroupsDisplay event=\{detail\}/)
    assert.match(routing, /event\.channel_groups/)
    assert.match(routing, /event\.group_name/)
    assert.match(routing, /event\.group_id/)
    assert.match(routing, /getAuditRouteGroupReference/)
    assert.match(routing, /getAuditChannelGroupReferences/)
    assert.match(routing, /kind:\s*'unassigned'/)
    assert.match(routing, /kind:\s*'historical'/)
  })

  test('renders the full prompt context online in the event detail', () => {
    const events = readSource('events-view.tsx')

    assert.match(events, /from '\@\/components\/ui\/markdown'/)
    assert.match(events, /<Markdown[\s\S]*breaks/)
    assert.match(events, /max-h-\[52vh\]/)
    assert.match(events, /<TabsTrigger value='all'>\{t\('All output'\)\}/)
    assert.match(events, /<TabsTrigger value='client'>/)
    assert.match(events, /<TabsTrigger value='llm'>/)
    assert.match(events, /Client → LLM/)
    assert.match(events, /LLM → client/)
    assert.match(events, /overflow-y-auto/)
  })

  test('shows and safely highlights matched sensitive keywords', () => {
    const events = readSource('events-view.tsx')
    const types = readSource('types.ts')

    assert.match(types, /matched_keywords\?: string\[\]/)
    assert.match(events, /t\('Matched keywords'\)/)
    assert.match(events, /detail\.matched_keywords/)
    assert.match(events, /createKeywordHighlightPlugin/)
    assert.match(
      readSource('matched-keyword-highlight.ts'),
      /data-audit-keyword-highlight/
    )
    assert.doesNotMatch(events, /dangerouslySetInnerHTML/)
  })

  test('keeps Classic audit context direction filters in sync', () => {
    const events = readClassicSource('pages', 'SecurityAudit', 'EventsTab.jsx')

    assert.match(events, /<Tabs[\s\S]*itemKey='all'/)
    assert.match(events, /itemKey='client'/)
    assert.match(events, /itemKey='llm'/)
    assert.match(events, /客户端 → LLM/)
    assert.match(events, /LLM → 客户端/)
    assert.match(events, /max-h-\[52vh\]/)
    assert.match(events, /overflow-y-auto/)
  })

  test('saves migrated sensitive-word rules atomically', () => {
    const view = readSource('builtin-policy-view.tsx')
    const systemApi = readSource('..', 'system-settings', 'api.ts')
    const editor = readSource(
      '..',
      'system-settings',
      'request-limits',
      'sensitive-words-section.tsx'
    )
    const saveSection = view.slice(
      view.indexOf('const savePolicy = async'),
      view.indexOf('if (policyQuery.isError)')
    )

    assert.match(view, /sensitive_rules:\s*values\.SensitiveRules/)
    assert.match(view, /sensitive_rule_channel_ids:/)
    assert.match(saveSection, /await updateSecurityAuditBuiltinPolicy/)
    assert.doesNotMatch(saveSection, /runSensitive/)
    assert.match(editor, /onSaveValues/)
    assert.match(editor, /inlineActions/)
    assert.match(systemApi, /\/api\/security-audit\/builtin-policy\/channels/)
    assert.match(systemApi, /\/api\/security-audit\/builtin-policy\/groups/)
    assert.match(editor, /channel\.id > 0/)
    assert.match(editor, /getSensitiveRuleGroups/)
    assert.match(editor, /TARGET_GROUPS/)
    assert.match(editor, /TARGET_ALL/)
    assert.match(editor, /channelsQuery\.isError/)
    assert.match(editor, /groupsQuery\.refetch/)
    assert.match(editor, /TARGET_CHANNELS/)
    assert.match(editor, /groupCodes/)
    assert.doesNotMatch(editor, /Keyword group references/)
    assert.doesNotMatch(editor, /selectedChannelIds/)
    assert.doesNotMatch(editor, /getUpstreamChannels/)
    assert.doesNotMatch(editor, /\}, \[defaultValues\]\)/)
  })

  test('does not require generic identity verification inside security audit', () => {
    const page = readSource('index.tsx')
    const endpoints = readSource('endpoints-view.tsx')
    const events = readSource('events-view.tsx')

    assert.doesNotMatch(
      page,
      /SecureVerification|useSecureVerification|withVerification/
    )
    assert.doesNotMatch(
      endpoints,
      /SensitiveActionRunner|runSensitive|withVerification/
    )
    assert.doesNotMatch(
      events,
      /SensitiveActionRunner|runSensitive|withVerification/
    )
  })

  test('keeps complete request archiving on an independent write-only contract', () => {
    const api = readSource('api.ts')
    const page = readSource('index.tsx')
    const view = readSource('request-archive-view.tsx')
    const types = readSource('types.ts')
    const saveSection = view.slice(
      view.indexOf('const save = async'),
      view.indexOf('const probe = async')
    )
    const probeSection = view.slice(
      view.indexOf('const probe = async'),
      view.indexOf('if (configQuery.isLoading')
    )

    assert.match(api, /request-archive\/config/)
    assert.match(api, /request-archive\/runtime/)
    assert.match(api, /request-archive\/targets\/probe/)
    assert.match(page, /value:\s*'request-archive'/)
    assert.match(view, /type='password'/)
    assert.match(
      view,
      /const result = await probeRequestArchiveTarget\(target\)/
    )
    assert.match(saveSection, /await updateRequestArchiveConfig/)
    assert.doesNotMatch(saveSection, /runSensitive/)
    assert.doesNotMatch(probeSection, /runSensitive/)
    assert.doesNotMatch(view, /Verify archive storage probe/)
    assert.doesNotMatch(
      view,
      /Confirm your identity before sending a connectivity probe to this archive storage target\./
    )
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

  test('keeps Classic channel options and configuration saves on the same contract', () => {
    const editor = readClassicSource(
      'pages',
      'Setting',
      'Operation',
      'SettingsSensitiveWords.jsx'
    )
    const builtinPolicy = readClassicSource(
      'pages',
      'SecurityAudit',
      'BuiltinPolicyTab.jsx'
    )
    const requestArchive = readClassicSource(
      'pages',
      'SecurityAudit',
      'RequestArchiveTab.jsx'
    )
    const builtinSave = builtinPolicy.slice(
      builtinPolicy.indexOf('const savePolicy = async'),
      builtinPolicy.indexOf('if (loadError)')
    )
    const archiveSave = requestArchive.slice(
      requestArchive.indexOf('const save ='),
      requestArchive.indexOf('const probe =')
    )

    assert.match(editor, /\/api\/security-audit\/builtin-policy\/channels/)
    assert.match(editor, /\/api\/security-audit\/builtin-policy\/groups/)
    assert.match(editor, /channel\.id > 0/)
    assert.match(editor, /channel\?\.tag\?\.trim\(\)/)
    assert.match(editor, /TARGET_GROUPS/)
    assert.match(editor, /TARGET_ALL/)
    assert.match(editor, /group_codes:/)
    assert.match(builtinSave, /await updateSecurityAuditBuiltinPolicy/)
    assert.match(builtinPolicy, /cyber_policy_auto_ban_enabled/)
    assert.match(builtinPolicy, /cyber_policy_ban_threshold/)
    assert.match(builtinPolicy, /cyber_policy_violation_window_hours/)
    assert.match(
      builtinPolicy,
      /enabled\s*\|\|\s*current\.upstream_policy_enabled/
    )
    assert.doesNotMatch(builtinSave, /runSensitive/)
    assert.match(archiveSave, /updateRequestArchiveConfig/)
    assert.doesNotMatch(archiveSave, /runSensitive/)
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
