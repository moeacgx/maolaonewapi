import assert from 'node:assert/strict'
import { readFile } from 'node:fs/promises'
import { test } from 'node:test'

const moduleUrl = new URL(
  '../src/pages/SecurityAudit/load-data.js',
  import.meta.url,
).href

test('安全审计配置成功时，运行状态或分组失败不阻塞页面数据', async () => {
  const { loadSecurityAuditData } = await import(`${moduleUrl}?test=${Date.now()}`)
  const result = await loadSecurityAuditData({
    getConfig: async () => ({ config_version: 7 }),
    getRuntime: async () => {
      throw new Error('runtime unavailable')
    },
    getGroups: async () => {
      throw new Error('groups unavailable')
    },
  })

  assert.deepEqual(result.config, { config_version: 7 })
  const runtime = await result.runtime
  const groups = await result.groups
  assert.equal(runtime.value, null)
  assert.equal(runtime.error?.message, 'runtime unavailable')
  assert.deepEqual(groups.value, [])
  assert.equal(groups.error?.message, 'groups unavailable')
})

test('安全审计配置不等待运行状态和分组加载', async () => {
  const { loadSecurityAuditData } = await import(`${moduleUrl}?pending=${Date.now()}`)
  let resolveRuntime
  let resolveGroups
  const result = await loadSecurityAuditData({
    getConfig: async () => ({ config_version: 8 }),
    getRuntime: () => new Promise((resolve) => {
      resolveRuntime = resolve
    }),
    getGroups: () => new Promise((resolve) => {
      resolveGroups = resolve
    }),
  })

  assert.deepEqual(result.config, { config_version: 8 })
  resolveRuntime({ process_status: 'running' })
  resolveGroups([{ id: 1, code: 'default', name: 'Default' }])
  assert.deepEqual(await result.runtime, {
    value: { process_status: 'running' },
    error: null,
  })
  assert.deepEqual(await result.groups, {
    value: [{ id: 1, code: 'default', name: 'Default' }],
    error: null,
  })
})

test('安全审计配置加载失败时不启动运行状态或分组请求', async () => {
  const { loadSecurityAuditData } = await import(`${moduleUrl}?config-error=${Date.now()}`)
  let runtimeCalled = false
  let groupsCalled = false

  await assert.rejects(
    loadSecurityAuditData({
      getConfig: async () => {
        throw new Error('config unavailable')
      },
      getRuntime: async () => {
        runtimeCalled = true
      },
      getGroups: async () => {
        groupsCalled = true
      },
    }),
    /config unavailable/,
  )
  assert.equal(runtimeCalled, false)
  assert.equal(groupsCalled, false)
})

test('保存安全审计配置时会使进行中的旧配置刷新失效', async () => {
  const source = await readFile(
    new URL('../src/pages/SecurityAudit/index.jsx', import.meta.url),
    'utf8',
  )
  assert.match(
    source,
    /setSaving\(true\);\s*loadRequestRef\.current \+= 1;/,
  )
})
