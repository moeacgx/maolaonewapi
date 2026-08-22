/*
Copyright (C) 2025 QuantumNous

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

import React from 'react';
import { parseTiersFromExpr, getCurrencyConfig } from '../../../../../helpers';
import { BILLING_PRICING_VARS } from '../../../../../constants';
import {
  splitBillingExprAndRequestRules,
  tryParseRequestRuleExpr,
  SOURCE_TIME,
  MATCH_RANGE,
  MATCH_EQ,
  MATCH_GTE,
  MATCH_LT,
  MATCH_CONTAINS,
  MATCH_EXISTS,
} from '../../../../../pages/Setting/Ratio/components/requestRuleExpr';

const VAR_LABELS = { p: '输入', c: '输出' };
const OP_LABELS = { '<': '<', '<=': '≤', '>': '>', '>=': '≥' };
const TIME_FUNC_LABELS = {
  hour: '小时',
  minute: '分钟',
  weekday: '星期',
  month: '月份',
  day: '日期',
};

function formatTokenHint(value) {
  const number = Number(value);
  if (!Number.isFinite(number) || number === 0) return '';
  if (number >= 1000000) {
    return `${(number / 1000000).toFixed(number % 1000000 === 0 ? 0 : 1)}M`;
  }
  if (number >= 1000) {
    return `${(number / 1000).toFixed(number % 1000 === 0 ? 0 : 1)}K`;
  }
  return String(number);
}

function formatConditionSummary(conditions, t) {
  return conditions
    .map((condition) => {
      if (!condition.var || !condition.op) return '';
      const variable = t(VAR_LABELS[condition.var] || condition.var);
      const hint = formatTokenHint(condition.value);
      return `${variable} ${OP_LABELS[condition.op] || condition.op} ${
        hint || condition.value
      }`;
    })
    .filter(Boolean)
    .join(' && ');
}

function describeCondition(condition, t) {
  if (condition.source === SOURCE_TIME) {
    const timeFunction = t(
      TIME_FUNC_LABELS[condition.timeFunc] || condition.timeFunc,
    );
    const timezone = condition.timezone || 'UTC';
    if (condition.mode === MATCH_RANGE) {
      return `${timeFunction} ${condition.rangeStart}:00~${
        condition.rangeEnd
      }:00 (${timezone})`;
    }
    const operator = { [MATCH_EQ]: '=', [MATCH_GTE]: '≥', [MATCH_LT]: '<' };
    return `${timeFunction} ${operator[condition.mode] || '='} ${
      condition.value
    } (${timezone})`;
  }

  const source = condition.source === 'header' ? t('请求头') : t('请求参数');
  if (condition.mode === MATCH_EXISTS) {
    return `${source} ${condition.path || ''} ${t('存在')}`;
  }
  if (condition.mode === MATCH_CONTAINS) {
    return `${source} ${condition.path || ''} ${t('包含')} "${
      condition.value
    }"`;
  }
  const operator = { eq: '=', gt: '>', gte: '≥', lt: '<', lte: '≤' };
  return `${source} ${condition.path || ''} ${
    operator[condition.mode] || '='
  } ${condition.value}`;
}

function describeGroup(group, t) {
  return (group.conditions || [])
    .map((condition) => describeCondition(condition, t))
    .filter(Boolean)
    .join(' && ');
}

export default function DynamicPricingBreakdown({ billingExpr, t }) {
  const { symbol, rate } = getCurrencyConfig();
  const { billingExpr: baseExpr, requestRuleExpr: ruleExpr } =
    splitBillingExprAndRequestRules(billingExpr || '');
  const tiers = parseTiersFromExpr(baseExpr);
  const ruleGroups = tryParseRequestRuleExpr(ruleExpr || '');
  const hasTiers = Array.isArray(tiers) && tiers.length > 0;
  const hasRules = Array.isArray(ruleGroups) && ruleGroups.length > 0;
  const priceFields = BILLING_PRICING_VARS.filter(
    (variable) => hasTiers && tiers.some((tier) => tier[variable.field] > 0),
  );

  return (
    <div className='classic-pricing-detail-dynamic-pricing'>
      <h4 className='classic-pricing-detail-subsection-title'>
        {t('动态计费')}
      </h4>

      {!hasTiers && !hasRules && (
        <code className='classic-pricing-detail-expression'>{billingExpr}</code>
      )}

      {hasTiers && (
        <div className='classic-pricing-detail-dynamic-block'>
          <span className='classic-pricing-detail-table-caption'>
            {t('分档价格表')}
          </span>
          <div className='classic-pricing-detail-table-wrap'>
            <table className='classic-pricing-detail-table'>
              <thead>
                <tr>
                  <th>{t('档位')}</th>
                  {priceFields.map((variable) => (
                    <th key={variable.field}>
                      {t(variable.shortLabel)} ({symbol}/1M tokens)
                    </th>
                  ))}
                </tr>
              </thead>
              <tbody>
                {tiers.map((tier, index) => (
                  <tr key={`${tier.label || 'default'}-${index}`}>
                    <td>
                      <div className='classic-pricing-detail-tier-name'>
                        <span className='classic-pricing-detail-pill'>
                          {tier.label || t('默认')}
                        </span>
                        {tier.conditions?.length > 0 && (
                          <span className='classic-pricing-detail-tier-condition'>
                            {formatConditionSummary(tier.conditions, t)}
                          </span>
                        )}
                      </div>
                    </td>
                    {priceFields.map((variable) => (
                      <td
                        key={variable.field}
                        className='classic-pricing-detail-table-number'
                      >
                        {tier[variable.field] > 0
                          ? `${symbol}${(tier[variable.field] * rate).toFixed(
                              4,
                            )}`
                          : '—'}
                      </td>
                    ))}
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        </div>
      )}

      {hasRules && (
        <div className='classic-pricing-detail-dynamic-block'>
          <span className='classic-pricing-detail-table-caption'>
            {t('条件乘数')}
          </span>
          <div className='classic-pricing-detail-rule-list'>
            {ruleGroups.map((group, index) => (
              <div
                key={`${group.multiplier}-${index}`}
                className='classic-pricing-detail-rule-row'
              >
                <span>{describeGroup(group, t)}</span>
                <strong>{group.multiplier}x</strong>
              </div>
            ))}
          </div>
        </div>
      )}
    </div>
  );
}
