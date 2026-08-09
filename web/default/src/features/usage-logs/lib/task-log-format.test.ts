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
import { getTaskLogModelDisplay } from './task-log-format.ts'

test('任务日志模型展示优先使用请求模型并标注实际上游模型', () => {
  assert.deepEqual(
    getTaskLogModelDisplay({
      properties: {
        origin_model_name: 'gpt-image-1',
        upstream_model_name: 'openai/gpt-image-1',
      },
    } as never),
    {
      requestModel: 'gpt-image-1',
      actualModel: 'openai/gpt-image-1',
    }
  )
})

test('任务日志模型展示兼容只记录上游模型的旧记录', () => {
  assert.deepEqual(
    getTaskLogModelDisplay({
      properties: {
        upstream_model_name: 'sora-2',
      },
    } as never),
    {
      requestModel: 'sora-2',
      actualModel: undefined,
    }
  )
})
