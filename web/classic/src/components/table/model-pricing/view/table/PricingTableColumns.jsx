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
import { Tag, Tooltip } from '@douyinfe/semi-ui';
import { IconHelpCircle } from '@douyinfe/semi-icons';
import {
  calculateModelPrice,
  getGroupDisplayName,
  getLobeHubIcon,
  getModelPriceItems,
  isModelPriceUnitSecond,
  stringToColor,
} from '../../../../../helpers';

const TOKEN_PRICE_KEYS = [
  'input',
  'completion',
  'input-ratio',
  'completion-ratio',
];
const CACHE_PRICE_KEYS = ['cache', 'cache-ratio'];

const formatPriceValue = (value) =>
  String(value)
    .replace(/(\.\d*?[1-9])0+$/u, '$1')
    .replace(/\.0+$/u, '');

const getBillingMode = (record, t) => {
  if (record.billing_mode === 'tiered_expr') {
    return {
      className: 'classic-pricing-billing-mode-warning',
      label: t('动态计费'),
    };
  }

  if (Number(record.quota_type) === 0) {
    return {
      className: 'classic-pricing-billing-mode-info',
      label: t('按量计费'),
    };
  }

  if (Number(record.quota_type) === 1) {
    return {
      className: isModelPriceUnitSecond(record.model_price_unit)
        ? 'classic-pricing-billing-mode-neutral'
        : 'classic-pricing-billing-mode-purple',
      label: t(
        isModelPriceUnitSecond(record.model_price_unit)
          ? '按秒计费'
          : '按次计费',
      ),
    };
  }

  return {
    className: 'classic-pricing-billing-mode-neutral',
    label: t('未知'),
  };
};

const renderBadgeList = (items, className = '') => {
  if (!items?.length) {
    return <span className='classic-pricing-table-empty'>-</span>;
  }

  const visibleItems = items.slice(0, 3);
  const hiddenCount = Math.max(items.length - visibleItems.length, 0);

  return (
    <div className={`classic-pricing-table-badge-list ${className}`}>
      {visibleItems.map((item) => (
        <Tag
          key={item}
          className='classic-pricing-table-badge'
          color={stringToColor(item)}
          shape='circle'
          size='small'
        >
          {item}
        </Tag>
      ))}
      {hiddenCount > 0 && (
        <span className='classic-pricing-table-badge-more'>+{hiddenCount}</span>
      )}
    </div>
  );
};

const renderPrice = (priceData, t, siteDisplayType) => {
  if (priceData.isDynamicPricing) {
    return (
      <span className='classic-pricing-table-dynamic-price'>
        {t('动态计费')}
      </span>
    );
  }

  const items = getModelPriceItems(priceData, t, siteDisplayType);
  const tokenItems = items.filter((item) =>
    TOKEN_PRICE_KEYS.includes(item.key),
  );

  if (priceData.isPerToken && tokenItems.length > 0) {
    return (
      <div className='classic-pricing-table-price'>
        <span className='classic-pricing-table-price-value'>
          {tokenItems.slice(0, 2).map((item, index) => (
            <React.Fragment key={item.key}>
              {index > 0 && (
                <span className='classic-pricing-table-price-separator'>/</span>
              )}
              {formatPriceValue(item.value)}
            </React.Fragment>
          ))}
        </span>
        <span className='classic-pricing-table-price-unit'>
          {priceData.isTokensDisplay ? t('倍率') : tokenItems[0].suffix}
        </span>
      </div>
    );
  }

  const item = items[0];
  if (!item) {
    return <span className='classic-pricing-table-empty'>-</span>;
  }

  return (
    <div className='classic-pricing-table-price'>
      <span className='classic-pricing-table-price-value'>
        {item.isVariantRange && `${t('起')} `}
        {formatPriceValue(item.minimumValue ?? item.value)}
      </span>
      <span className='classic-pricing-table-price-unit'>{item.suffix}</span>
    </div>
  );
};

const renderCachedPrice = (priceData, t, siteDisplayType) => {
  if (priceData.isDynamicPricing) {
    return <span className='classic-pricing-table-empty'>{t('动态计费')}</span>;
  }

  const item = getModelPriceItems(priceData, t, siteDisplayType).find((entry) =>
    CACHE_PRICE_KEYS.includes(entry.key),
  );

  if (!item) {
    return <span className='classic-pricing-table-empty'>-</span>;
  }

  return (
    <div className='classic-pricing-table-price'>
      <span className='classic-pricing-table-price-value'>
        {formatPriceValue(item.value)}
      </span>
      <span className='classic-pricing-table-price-unit'>
        {priceData.isTokensDisplay ? t('倍率') : item.suffix}
      </span>
    </div>
  );
};

export const getPricingTableColumns = ({
  t,
  selectedGroup,
  groupRatio,
  groupNames = {},
  copyText,
  setModalImageUrl,
  setIsModalOpenurl,
  currency,
  siteDisplayType,
  tokenUnit,
  displayPrice,
  showRatio,
}) => {
  const priceDataCache = new WeakMap();

  const getPriceData = (record) => {
    let priceData = priceDataCache.get(record);
    if (!priceData) {
      priceData = calculateModelPrice({
        record,
        selectedGroup,
        groupRatio,
        tokenUnit,
        displayPrice,
        currency,
        quotaDisplayType: siteDisplayType,
      });
      priceDataCache.set(record, priceData);
    }
    return priceData;
  };

  const modelNameColumn = {
    title: t('模型'),
    dataIndex: 'model_name',
    width: 254,
    sorter: (left, right) =>
      String(left.model_name || '').localeCompare(
        String(right.model_name || ''),
      ),
    render: (text, record) => {
      const icon = record.icon || record.vendor_icon;
      return (
        <button
          type='button'
          className='classic-pricing-table-model'
          title={text}
          onClick={(event) => {
            event.stopPropagation();
            copyText(text);
          }}
        >
          <span className='classic-pricing-table-model-icon'>
            {icon ? getLobeHubIcon(icon, 16) : text?.slice(0, 1).toUpperCase()}
          </span>
          <span className='classic-pricing-table-model-name'>{text}</span>
        </button>
      );
    },
    onFilter: (value, record) =>
      record.model_name.toLowerCase().includes(value.toLowerCase()),
  };

  const typeColumn = {
    title: t('类型'),
    dataIndex: 'quota_type',
    width: 95,
    render: (_, record) => {
      const billingMode = getBillingMode(record, t);
      return (
        <span
          className={`classic-pricing-billing-mode ${billingMode.className}`}
        >
          {billingMode.label}
        </span>
      );
    },
  };

  const priceColumn = {
    title: siteDisplayType === 'TOKENS' ? t('计费摘要') : t('价格'),
    dataIndex: 'model_price',
    width: 142,
    render: (_, record) =>
      renderPrice(getPriceData(record), t, siteDisplayType),
  };

  const cachedPriceColumn = {
    title: t('缓存'),
    dataIndex: 'cache_ratio',
    width: 87,
    render: (_, record) =>
      renderCachedPrice(getPriceData(record), t, siteDisplayType),
  };

  const vendorColumn = {
    title: t('供应商'),
    dataIndex: 'vendor_name',
    width: 118,
    render: (vendorName, record) => {
      if (!vendorName) {
        return <span className='classic-pricing-table-empty'>-</span>;
      }

      return (
        <Tag
          className='classic-pricing-table-vendor'
          color={stringToColor(vendorName)}
          shape='circle'
          size='small'
          prefixIcon={getLobeHubIcon(record.vendor_icon || 'Layers', 13)}
        >
          {vendorName}
        </Tag>
      );
    },
  };

  const tagsColumn = {
    title: t('标签'),
    dataIndex: 'tags',
    width: 137,
    render: (text) =>
      renderBadgeList(
        String(text || '')
          .split(/[,;|]+/)
          .map((tag) => tag.trim())
          .filter(Boolean),
      ),
  };

  const endpointColumn = {
    title: t('可用端点类型'),
    dataIndex: 'supported_endpoint_types',
    width: 196,
    render: (endpoints) => renderBadgeList(endpoints),
  };

  const groupsColumn = {
    title: t('分组'),
    dataIndex: 'enable_groups',
    width: 101,
    render: (groups) =>
      renderBadgeList(
        (groups || [])
          .filter((group) => group && group !== 'all')
          .map((group) => getGroupDisplayName(group, groupNames)),
      ),
  };

  const ratioColumn = {
    title: () => (
      <div className='classic-pricing-table-ratio-header'>
        <span>{t('倍率')}</span>
        <Tooltip content={t('倍率是为了方便换算不同价格的模型')}>
          <IconHelpCircle
            className='classic-pricing-table-ratio-help'
            onClick={(event) => {
              event.stopPropagation();
              setModalImageUrl('/ratio.png');
              setIsModalOpenurl(true);
            }}
          />
        </Tooltip>
      </div>
    ),
    dataIndex: 'model_ratio',
    width: 170,
    render: (modelRatio, record) => {
      const priceData = getPriceData(record);
      const completionRatio = Number(record.completion_ratio);
      return (
        <div className='classic-pricing-table-ratio-list'>
          <span>
            {t('模型')} {record.quota_type === 0 ? modelRatio : '-'}
          </span>
          <span>
            {t('补全')}{' '}
            {record.quota_type === 0 && Number.isFinite(completionRatio)
              ? formatPriceValue(completionRatio)
              : '-'}
          </span>
          <span>
            {t('分组')} {priceData.usedGroupRatio ?? '-'}
          </span>
        </div>
      );
    },
  };

  const columns = [
    modelNameColumn,
    typeColumn,
    priceColumn,
    cachedPriceColumn,
    vendorColumn,
    tagsColumn,
    endpointColumn,
    groupsColumn,
  ];

  if (showRatio) {
    columns.push(ratioColumn);
  }

  return columns;
};
