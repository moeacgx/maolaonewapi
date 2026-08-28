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
} from '../../../../../helpers';
import {
  getBillingDiscountText,
  getBillingFactors,
  hasBillingPriceAdjustment,
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

const PriceCards = ({ items, t }) => {
  if (items.length === 0) return null;

  const primaryItems = items.filter(
    (item) => item.key === 'input' || item.key === 'completion',
  );
  const secondaryItems = items.filter(
    (item) => item.key !== 'input' && item.key !== 'completion',
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
    // 规格规则只在“规格价格”区展示，基础价格区仅保留范围/兜底摘要。
    includeVariantRules: false,
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
        {isDynamic ? (
          <p className='classic-pricing-detail-table-muted'>
            {t('此模型采用动态计费，价格明细见下方。')}
          </p>
        ) : (
          <PriceCards items={basePriceItems} t={t} />
        )}
      </section>

      <SpecificationPricing
        modelData={modelData}
        basePriceItems={basePriceItems}
        displayPrice={displayPrice}
        t={t}
      />

      {isDynamic && modelData.billing_expr && (
        <section className='classic-pricing-detail-pricing-section'>
          <DynamicPricingBreakdown billingExpr={modelData.billing_expr} t={t} />
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
                <span
                  className='classic-pricing-detail-pill classic-pricing-detail-group-pill'
                  style={{
                    '--classic-pricing-group-color': getGroupTextColor(group),
                  }}
                >
                  {getGroupDisplayName(group, groupNames)}
                </span>
                {index < autoChain.length - 1 && (
                  <span aria-hidden='true'>→</span>
                )}
              </React.Fragment>
            ))}
          </div>
        )}

        {groupRows.length === 0 ? (
          <p className='classic-pricing-detail-table-muted'>
            {t('当前没有可用分组价格信息')}
          </p>
        ) : (
          <>
            <div className='classic-pricing-detail-table-wrap classic-pricing-detail-group-table-wrap'>
              <table className='classic-pricing-detail-table'>
                <thead>
                  <tr>
                    <th>{t('分组')}</th>
                    <th>{t('倍率')}</th>
                    {isDynamic ? (
                      <th>{t('动态计费')}</th>
                    ) : (
                      groupPriceColumns.map((item) => (
                        <th key={item.key}>
                          {getPriceItemLabel(item, t, true)}
                        </th>
                      ))
                    )}
                  </tr>
                </thead>
                <tbody>
                  {groupRows.map((row) => {
                    const priceByKey = new Map(
                      row.priceItems.map((item) => [item.key, item]),
                    );
                    const hasPriceAdjustment = hasBillingPriceAdjustment(
                      row.discountFactor,
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
                            {hasPriceAdjustment && (
                              <span className='classic-pricing-detail-discount-badge'>
                                {getBillingDiscountText(row.discountFactor, t)}
                              </span>
                            )}
                          </div>
                        </td>
                        <td className='classic-pricing-detail-table-number'>
                          {row.ratio}x
                        </td>
                        {isDynamic ? (
                          <td className='classic-pricing-detail-table-muted'>
                            {t('见上方动态计费详情')}
                          </td>
                        ) : (
                          groupPriceColumns.map((column) => (
                            <td
                              key={column.key}
                              className='classic-pricing-detail-table-number'
                            >
                              {formatPriceValue(priceByKey.get(column.key))}
                            </td>
                          ))
                        )}
                      </tr>
                    );
                  })}
                </tbody>
              </table>
            </div>
            {modelData.quota_type === 0 && (
              <p className='classic-pricing-detail-price-footnote'>
                {t('价格显示单位')} 1{tokenUnit} tokens
              </p>
            )}
          </>
        )}
      </section>
    </section>
  );
};

export default ModelPricingTable;
