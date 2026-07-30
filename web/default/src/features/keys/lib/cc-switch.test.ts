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
import test from 'node:test'
import {
  buildCCSwitchURL,
  isCCSwitchPreset,
  isValidCCSwitchAddress,
  normalizeCCSwitchAddress,
  resolveCCSwitchAddresses,
  withCCSwitchAPIAddress,
} from './cc-switch.ts'

function parseImportURL(url: string) {
  return new URLSearchParams(url.slice(url.indexOf('?') + 1))
}

test('CC Switch API 地址去除空白和末尾斜杠', () => {
  assert.equal(
    normalizeCCSwitchAddress('  https://api.example.com///  '),
    'https://api.example.com'
  )
})

test('保存后的规范化地址立即覆盖状态缓存中的旧值', () => {
  const updated = withCCSwitchAPIAddress(
    {
      cc_switch_api_address: 'https://old.example.com',
      server_address: 'https://www.example.com',
      data: {
        cc_switch_api_address: 'https://older.example.com',
        server_address: 'https://www.example.com',
      },
    },
    '  https://new.example.com/gateway///  '
  )

  assert.equal(updated.cc_switch_api_address, 'https://new.example.com/gateway')
  assert.equal(
    updated.data?.cc_switch_api_address,
    'https://new.example.com/gateway'
  )
  assert.deepEqual(resolveCCSwitchAddresses(updated), {
    apiAddress: 'https://new.example.com/gateway',
    homepage: 'https://www.example.com',
  })
})

test('清空自定义地址时覆盖旧值并回退网站地址', () => {
  const updated = withCCSwitchAPIAddress(
    {
      cc_switch_api_address: 'https://old.example.com',
      server_address: 'https://www.example.com',
    },
    '   '
  )

  assert.equal(updated.cc_switch_api_address, '')
  assert.deepEqual(resolveCCSwitchAddresses(updated), {
    apiAddress: 'https://www.example.com',
    homepage: 'https://www.example.com',
  })
})

test('CC Switch API 地址仅接受无凭据和附加参数的 HTTP 绝对地址', () => {
  assert.equal(isValidCCSwitchAddress(''), true)
  assert.equal(isValidCCSwitchAddress('https://api.example.com/gateway'), true)
  assert.equal(isValidCCSwitchAddress('api.example.com'), false)
  assert.equal(isValidCCSwitchAddress('ftp://api.example.com'), false)
  assert.equal(isValidCCSwitchAddress('https://user:pass@example.com'), false)
  assert.equal(isValidCCSwitchAddress('https://api.example.com?token=x'), false)
})

test('自定义 API 地址优先且 homepage 始终保留网站地址', () => {
  assert.deepEqual(
    resolveCCSwitchAddresses(
      {
        cc_switch_api_address: 'https://api.example.com/',
        server_address: 'https://www.example.com/',
      },
      'https://fallback.example.com'
    ),
    {
      apiAddress: 'https://api.example.com',
      homepage: 'https://www.example.com',
    }
  )
})

test('支持状态接口 data 包装并在空配置时逐级回退', () => {
  assert.deepEqual(
    resolveCCSwitchAddresses(
      {
        data: {
          cc_switch_api_address: 'https://api.example.com/',
          server_address: 'https://www.example.com/',
        },
      },
      'https://fallback.example.com'
    ),
    {
      apiAddress: 'https://api.example.com',
      homepage: 'https://www.example.com',
    }
  )
  assert.deepEqual(
    resolveCCSwitchAddresses({}, 'https://fallback.example.com/'),
    {
      apiAddress: 'https://fallback.example.com',
      homepage: 'https://fallback.example.com',
    }
  )
})

test('Codex 幂等补充 /v1，Claude 使用 API 根地址', () => {
  const common = {
    name: 'Provider',
    models: { model: 'gpt-test' },
    apiKey: 'sk-test',
    status: {
      cc_switch_api_address: 'https://api.example.com/',
      server_address: 'https://www.example.com/',
    },
    origin: 'https://fallback.example.com',
  }

  const codex = parseImportURL(buildCCSwitchURL({ ...common, app: 'codex' }))
  const claude = parseImportURL(buildCCSwitchURL({ ...common, app: 'claude' }))
  assert.equal(codex.get('endpoint'), 'https://api.example.com/v1')
  assert.equal(claude.get('endpoint'), 'https://api.example.com')
  assert.equal(codex.get('homepage'), 'https://www.example.com')

  const existingV1 = parseImportURL(
    buildCCSwitchURL({
      ...common,
      app: 'codex',
      status: {
        ...common.status,
        cc_switch_api_address: 'https://api.example.com/v1/',
      },
    })
  )
  assert.equal(existingV1.get('endpoint'), 'https://api.example.com/v1')
})

test('聊天配置中的 ccswitch 标识统一识别为 CCS 弹窗入口', () => {
  assert.equal(isCCSwitchPreset('ccswitch'), true)
  assert.equal(isCCSwitchPreset('CCSwitch://open'), true)
  assert.equal(isCCSwitchPreset('https://ccswitch.io'), false)
})
