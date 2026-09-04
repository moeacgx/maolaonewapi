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
*/
import { describe, expect, test } from 'vitest'

import { getUpstreamResponseModelName, parseLogOther } from '../format'

describe('upstream response model usage log field', () => {
  test('returns the trimmed upstream response model from parsed other data', () => {
    const other = parseLogOther(
      '{"upstream_response_model_name":" provider-actual "}'
    )

    expect(getUpstreamResponseModelName(other)).toBe('provider-actual')
  })

  test('returns undefined when the upstream response model is absent or blank', () => {
    expect(getUpstreamResponseModelName(parseLogOther('{}'))).toBeUndefined()
    expect(
      getUpstreamResponseModelName(
        parseLogOther('{"upstream_response_model_name":"  "}')
      )
    ).toBeUndefined()
  })
})
