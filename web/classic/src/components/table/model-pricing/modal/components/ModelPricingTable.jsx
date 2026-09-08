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
import {
  calculateModelPrice,
  getGroupDisplayName,
  getModelPriceVariantRange,
  getModelPriceVariantRuleLabel,
  getModelPriceItems,
  parseTiersFromExpr,
} from '../../../../../helpers';
import { BILLING_PRICING_VARS } from '../../../../../constants';
import { splitBillingExprAndRequestRules } from '../../../../../pages/Setting/Ratio/components/requestRuleExpr';
import {
  formatDynamicUnitPrice,
  getBillingDiscountText,
  getBillingFactors,
  getDynamicFormattedPricesByTier,
  getDynamicPriceFieldsFromTiers,
  hasBillingPriceAdjustment,
  isPrimaryDynamicPriceField,
} from '../../billing/utils';
import { getGroupTextColor } from '../../groupVisuals';
import DynamicPricingBreakdown from './DynamicPricingBreakdown';

const getPriceItemLabel = (item, t, compact = false) => {
  const labels = {
    input: '输入',
    completion: '输出',
    cache: '缓存输入',
    'create-cache': '缓存写入',
    image: '图片输入',
    'audio-input': '音频输入',
    'audio-output': '音频输出',
    'input-ratio': '输入',
    'completion-ratio': '输出',
    'cache-ratio': '缓存输入',
    'create-cache-ratio': '缓存写入',
    'image-ratio': '图片输入',
    'audio-input-ratio': '音频输入',
    'audio-output-ratio': '音频输出',
    fixed: '价格',
    'fixed-fallback': '固定价格',
    'fixed-variant-range': '规格价格',
  };
  const compactLabels = {
    cache: '缓存',
    'create-cache': '缓存写入',
    'cache-ratio': '缓存',
    'create-cache-ratio': '缓存写入',
  };

  if (compact && compactLabels[item.key]) {
    return t(compactLabels[item.key]);
  }

  return t(labels[item.key] || item.label);
};

const formatPriceText = (value) =>
  String(value ?? '—')
    .replace(/(\d+\.\d*?[1-9])0+(?=(?:\s|$))/gu, '$1')
    .replace(/(\d+)\.0+(?=(?:\s|$))/gu, '$1');

const formatPriceUnit = (unit) =>
  String(unit || '')
    .replace(/\s*Tokens?\b/giu, '')
    .trim();

const formatPriceValue = (item) => {
  if (!item || item.isDynamic) return '—';
  return formatPriceText(item.value);
};

const getDiscountBadgeClass = (factor) => {
  if (factor > 1) {
    return 'classic-pricing-detail-discount-badge classic-pricing-detail-discount-badge-up';
  }
  return 'classic-pricing-detail-discount-badge';
};

const SpecialExpressionNotice = ({ title, description, expression, t }) => (
  <div className='classic-pricing-detail-special-expr'>
    <div className='classic-pricing-detail-special-expr-title'>{title}</div>
    {description && <p>{description}</p>}
    <span className='classic-pricing-detail-table-caption'>
      {t('原始表达式')}
    </span>
    <code className='classic-pricing-detail-expression'>{expression}</code>
  </div>
);

const GroupDiscountBadge = ({ factor, t }) => {
  if (!hasBillingPriceAdjustment(factor)) return null;
  return (
    <span className={getDiscountBadgeClass(factor)}>
      {getBillingDiscountText(factor, t)}
    </span>
  );
};

const DynamicGroupPricingCards = ({
  groupRows,
  groupNames,
  dynamicTiers,
  dynamicPriceFields,
  dynamicPriceOptions,
  tokenUnitLabel,
  t,
}) => (
  <div className='classic-pricing-detail-group-cards'>
    {groupRows.map((row) => {
      const formattedTiers = getDynamicFormattedPricesByTier(
        dynamicTiers,
        dynamicPriceFields,
        {
          groupRatio: row.ratio,
          ...dynamicPriceOptions,
        },
      );

      return (
        <div key={row.group} className='classic-pricing-detail-group-card'>
          <div className='classic-pricing-detail-group-card-header'>
            <div
              className='classic-pricing-detail-group-cell'
              style={{
                '--classic-pricing-group-color': getGroupTextColor(row.group),
              }}
            >
              <span className='classic-pricing-detail-group-link'>
                {getGroupDisplayName(row.group, groupNames)}
              </span>
              <GroupDiscountBadge factor={row.discountFactor} t={t} />
            </div>
            <strong className='classic-pricing-detail-group-card-ratio'>
              {row.ratio}x
            </strong>
          </div>
          <div className='classic-pricing-detail-table-wrap'>
            <table className='classic-pricing-detail-table'>
              <thead>
                <tr>
                  <th>{t('档位')}</th>
                  {dynamicPriceFields.map((variable) => (
                    <th key={variable.field}>{t(variable.shortLabel)}</th>
                  ))}
                </tr>
              </thead>
              <tbody>
                {formattedTiers.map(({ tier, prices }, index) => (
                  <tr key={`${row.group}-${tier.label || 'default'}-${index}`}>
                    <td className='classic-pricing-detail-table-muted'>
                      {tier.label || t('默认')}
                    </td>
                    {dynamicPriceFields.map((variable) => (
                      <td
                        key={variable.field}
                        className='classic-pricing-detail-table-number'
                      >
                        {prices[variable.field] || '—'}
                      </td>
                    ))}
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        </div>
      );
    })}
    <p className='classic-pricing-detail-price-footnote'>
      {t('价格显示单位')} {tokenUnitLabel} tokens
    </p>
  </div>
);

const PriceCards = ({ items, t }) => {
  if (items.length === 0) return null;

  const primaryItems = items.filter(
    (item) =>
      item.key === 'input' ||
      item.key === 'completion' ||
      isPrimaryDynamicPriceField(item.key),
  );
  const secondaryItems = items.filter(
    (item) =>
      item.key !== 'input' &&
      item.key !== 'completion' &&
      !isPrimaryDynamicPriceField(item.key),
  );
  const cardItems = primaryItems.length > 0 ? primaryItems : secondaryItems;
  const listItems = primaryItems.length > 0 ? secondaryItems : [];

  return (
    <div className='classic-pricing-detail-price-cards'>
      <div className='classic-pricing-detail-primary-price-grid'>
        {cardItems.map((item) => (
          <div
            key={item.key}
            className='classic-pricing-detail-price-card-item'
          >
            <span className='classic-pricing-detail-price-label'>
              {getPriceItemLabel(item, t)}
            </span>
            <strong className='classic-pricing-detail-price-amount'>
              {formatPriceText(item.value)}
              {item.suffix && (
                <span className='classic-pricing-detail-price-unit'>
                  {formatPriceUnit(item.suffix)}
                </span>
              )}
            </strong>
          </div>
        ))}
      </div>
      {listItems.length > 0 && (
        <div className='classic-pricing-detail-secondary-price-list'>
          {listItems.map((item) => (
            <div
              key={item.key}
              className='classic-pricing-detail-secondary-price-row'
            >
              <span className='classic-pricing-detail-price-label'>
                {getPriceItemLabel(item, t)}
              </span>
              <strong className='classic-pricing-detail-price-amount'>
                {formatPriceText(item.value)}
                {item.suffix && (
                  <span className='classic-pricing-detail-price-unit'>
                    {formatPriceUnit(item.suffix)}
                  </span>
                )}
              </strong>
            </div>
          ))}
        </div>
      )}
    </div>
  );
};

const SpecificationPricing = ({
  modelData,
  basePriceItems,
  displayPrice,
  t,
}) => {
  if (
    modelData.quota_type !== 1 ||
    (modelData.billing_mode === 'tiered_expr' && modelData.billing_expr)
  ) {
    return null;
  }

  const configurations = [
    { route: '', config: modelData.model_price_variants },
    ...Object.entries(modelData.model_route_price_variants || {}).map(
      ([route, config]) => ({ route, config }),
    ),
  ]
    .map(({ route, config }) => {
      const range = getModelPriceVariantRange(modelData.model_price, config);
      if (!range) return null;
      return { route, config: range.config, rules: range.rules };
    })
    .filter(Boolean);

  if (configurations.length === 0) return null;

  const unit = formatPriceUnit(
    basePriceItems.find((item) => item.key.startsWith('fixed'))?.suffix,
  );

  return (
    <section className='classic-pricing-detail-pricing-section'>
      <h4 className='classic-pricing-detail-subsection-title'>
        {t('规格价格')}
      </h4>
      <div className='classic-pricing-detail-variant-routes'>
        {configurations.map(({ route, config, rules }) => (
          <section
            className='classic-pricing-detail-variant-route'
            key={route || 'default'}
          >
            <h5 className='classic-pricing-detail-variant-route-title'>
              {route || t('默认路由')}
            </h5>
            <div className='classic-pricing-detail-variant-route-rules'>
              {rules.map((rule) => (
                <div
                  className='classic-pricing-detail-rule-row'
                  key={`${rule.resolution || ''}:${rule.quality || ''}`}
                >
                  <span>{getModelPriceVariantRuleLabel(rule, config, t)}</span>
                  <strong>
                    {formatPriceText(
                      displayPrice ? displayPrice(rule.price) : rule.price,
                    )}
                    {unit && (
                      <span className='classic-pricing-detail-price-unit'>
                        {unit}
                      </span>
                    )}
                  </strong>
                </div>
              ))}
            </div>
          </section>
        ))}
      </div>
    </section>
  );
};

const ModelPricingTable = ({
  modelData,
  groupRatio = {},
  groupNames = {},
  currency,
  siteDisplayType,
  tokenUnit,
  displayPrice,
  priceRate,
  usdExchangeRate,
  usableGroup,
  autoGroups = [],
  t,
}) => {
  if (!modelData) return null;

  const isDynamic =
    modelData.billing_mode === 'tiered_expr' && Boolean(modelData.billing_expr);
  const { billingExpr: dynamicBillingExpr } = splitBillingExprAndRequestRules(
    modelData.billing_expr || '',
  );
  const dynamicTiers = isDynamic ? parseTiersFromExpr(dynamicBillingExpr) : [];
  const dynamicPriceFields = getDynamicPriceFieldsFromTiers(
    dynamicTiers,
    BILLING_PRICING_VARS,
  );
  const tokenUnitLabel = tokenUnit === 'K' ? '1K' : '1M';
  const dynamicPriceOptions = {
    tokenUnit,
    displayPrice,
  };
  const baseDynamicPriceItems = dynamicPriceFields
    .map((variable) => {
      const formatted = formatDynamicUnitPrice({
        valuePerMillionTokens: Number(dynamicTiers[0]?.[variable.field]),
        groupRatio: 1,
        ...dynamicPriceOptions,
      });
      if (!formatted) return null;
      let itemKey = variable.field;
      if (variable.key === 'p') itemKey = 'input';
      if (variable.key === 'c') itemKey = 'completion';
      return {
        key: itemKey,
        label: variable.shortLabel,
        value: formatted,
        suffix: `/ ${tokenUnitLabel}`,
      };
    })
    .filter(Boolean);
  const modelEnableGroups = Array.isArray(modelData.enable_groups)
    ? modelData.enable_groups
    : [];
  const availableGroups = Object.keys(usableGroup || {})
    .filter((group) => group && group !== 'auto')
    .filter((group) => modelEnableGroups.includes(group));
  const autoChain = autoGroups.filter((group) =>
    modelEnableGroups.includes(group),
  );
  const basePriceData = calculateModelPrice({
    record: modelData,
    selectedGroup: '_base',
    groupRatio: { ...groupRatio, _base: 1 },
    tokenUnit,
    displayPrice,
    currency,
    quotaDisplayType: siteDisplayType,
  });
  const basePriceItems = getModelPriceItems(basePriceData, t, siteDisplayType, {
    includeVariantRules: true,
  });
  const groupRows = availableGroups.map((group) => {
    const ratio = groupRatio[group] ?? 1;
    const priceData = calculateModelPrice({
      record: modelData,
      selectedGroup: group,
      groupRatio,
      tokenUnit,
      displayPrice,
      currency,
      quotaDisplayType: siteDisplayType,
    });
    const discountFactor = getBillingFactors({
      groupRatio: ratio,
      priceRate,
      usdExchangeRate,
    }).compositeFactor;

    return {
      group,
      ratio,
      discountFactor,
      priceItems: getModelPriceItems(priceData, t, siteDisplayType, {
        includeVariantRules: false,
      }),
    };
  });
  const groupPriceColumns = Array.from(
    new Map(
      groupRows
        .flatMap((row) => row.priceItems)
        .filter((item) => !item.isDynamic)
        .map((item) => [item.key, item]),
    ).values(),
  );

  return (
    <section className='classic-pricing-detail-price-card'>
      <h3 className='classic-pricing-detail-section-title'>{t('定价')}</h3>

      <section className='classic-pricing-detail-pricing-section'>
        <h4 className='classic-pricing-detail-subsection-title'>
          {t('基础价格')}
        </h4>
        {isDynamic && dynamicTiers.length === 0 ? (
          <SpecialExpressionNotice
            description={t('无法解析结构化价格')}
            expression={modelData.billing_expr}
            t={t}
            title={t('特殊计费表达式')}
          />
        ) : (
          <PriceCards
            items={isDynamic ? baseDynamicPriceItems : basePriceItems}
            t={t}
          />
        )}
      </section>

      <SpecificationPricing
        modelData={modelData}
        basePriceItems={basePriceItems}
        displayPrice={displayPrice}
        t={t}
      />

      {isDynamic && modelData.billing_expr && dynamicTiers.length > 0 && (
        <section className='classic-pricing-detail-pricing-section'>
          <DynamicPricingBreakdown
            billingExpr={modelData.billing_expr}
            displayPrice={displayPrice}
            t={t}
            tokenUnit={tokenUnit}
          />
        </section>
      )}

      <section className='classic-pricing-detail-pricing-section'>
        <h4 className='classic-pricing-detail-subsection-title'>
          {t('按分组定价')}
        </h4>
        {autoChain.length > 0 && (
          <div className='classic-pricing-detail-auto-chain'>
            <span>{t('自动分组链')}</span>
            <span aria-hidden='true'>→</span>
            {autoChain.map((group, index) => (
              <React.Fragment key={group}>
                <span className='classic-pricing-detail-pill'>
                  {getGroupDisplayName(group, groupNames)}
                </span>
                {index < autoChain.length - 1 && (
                  <span aria-hidden='true'>→</span>
                )}
              </React.Fragment>
            ))}
          </div>
        )}

        {groupRows.length === 0 && (
          <p className='classic-pricing-detail-table-muted'>
            {t('当前没有可用分组价格信息')}
          </p>
        )}
        {groupRows.length > 0 && isDynamic && dynamicTiers.length === 0 && (
          <SpecialExpressionNotice
            description={t(
              '该表达式不是标准分档计费表达式，无法展开分组价格。',
            )}
            expression={modelData.billing_expr}
            t={t}
            title={t('特殊计费表达式')}
          />
        )}
        {groupRows.length > 0 && isDynamic && dynamicTiers.length > 0 && (
          <DynamicGroupPricingCards
            dynamicPriceFields={dynamicPriceFields}
            dynamicPriceOptions={dynamicPriceOptions}
            dynamicTiers={dynamicTiers}
            groupNames={groupNames}
            groupRows={groupRows}
            t={t}
            tokenUnitLabel={tokenUnitLabel}
          />
        )}
        {groupRows.length > 0 && !isDynamic && (
          <>
            <div className='classic-pricing-detail-table-wrap classic-pricing-detail-group-table-wrap'>
              <table className='classic-pricing-detail-table'>
                <thead>
                  <tr>
                    <th>{t('分组')}</th>
                    <th>{t('倍率')}</th>
                    {groupPriceColumns.map((item) => (
                      <th key={item.key}>{getPriceItemLabel(item, t, true)}</th>
                    ))}
                  </tr>
                </thead>
                <tbody>
                  {groupRows.map((row) => {
                    const priceByKey = new Map(
                      row.priceItems.map((item) => [item.key, item]),
                    );

                    return (
                      <tr key={row.group}>
                        <td>
                          <div
                            className='classic-pricing-detail-group-cell'
                            style={{
                              '--classic-pricing-group-color':
                                getGroupTextColor(row.group),
                            }}
                          >
                            <span className='classic-pricing-detail-group-link'>
                              {getGroupDisplayName(row.group, groupNames)}
                            </span>
                            <GroupDiscountBadge
                              factor={row.discountFactor}
                              t={t}
                            />
                          </div>
                        </td>
                        <td className='classic-pricing-detail-table-number'>
                          {row.ratio}x
                        </td>
                        {groupPriceColumns.map((column) => (
                          <td
                            key={column.key}
                            className='classic-pricing-detail-table-number'
                          >
                            {formatPriceValue(priceByKey.get(column.key))}
                          </td>
                        ))}
                      </tr>
                    );
                  })}
                </tbody>
              </table>
            </div>
            {modelData.quota_type === 0 && (
              <p className='classic-pricing-detail-price-footnote'>
                {t('价格显示单位')} {tokenUnitLabel} tokens
              </p>
            )}
          </>
        )}
      </section>
    </section>
  );
};

export default ModelPricingTable;
