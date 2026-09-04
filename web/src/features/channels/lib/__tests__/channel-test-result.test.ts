/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as published by
the Free Software Foundation, either version 3 of the License, or
(at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program. If not, see <https://www.gnu.org/licenses/>.
*/
import { describe, expect, test } from 'vitest'

import { getChannelTestResponseModelName } from '../channel-actions'

describe('channel test upstream response model', () => {
  test('trims the model declared by the upstream response', () => {
    expect(
      getChannelTestResponseModelName({
        success: true,
        upstream_response_model_name: ' provider-actual ',
      })
    ).toBe('provider-actual')
  })

  test('returns undefined for missing or blank model names', () => {
    expect(getChannelTestResponseModelName({ success: true })).toBeUndefined()
    expect(
      getChannelTestResponseModelName({
        success: true,
        upstream_response_model_name: '  ',
      })
    ).toBeUndefined()
  })
})
