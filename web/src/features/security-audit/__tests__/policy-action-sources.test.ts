import { describe, expect, it } from 'vitest'

import { normalizeSecurityAuditPolicySources } from '../api'

describe('security audit policy action sources', () => {
  it('defaults missing values to cyber_policy for legacy responses', () => {
    expect(normalizeSecurityAuditPolicySources(undefined)).toEqual([
      'cyber_policy',
    ])
  })

  it('keeps an explicit empty selection and normalizes supported sources', () => {
    expect(normalizeSecurityAuditPolicySources([])).toEqual([])
    expect(
      normalizeSecurityAuditPolicySources([
        'biological_risk',
        'CYBER_POLICY',
        'unknown',
        'cyber_policy',
      ])
    ).toEqual(['cyber_policy', 'biological_risk'])
  })
})
