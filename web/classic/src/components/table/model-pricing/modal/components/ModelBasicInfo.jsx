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
import { HeartPulse, Timer } from 'lucide-react';
import {
  getGroupDisplayName,
  isModelPriceUnitSecond,
} from '../../../../../helpers';
import {
  formatLatency,
  formatSuccessRate,
  formatThroughput,
  getSuccessRateTextColor,
} from '../../performance/utils';

const getBillingType = (modelData, t) => {
  if (modelData?.billing_mode === 'tiered_expr') {
    return t('动态计费');
  }
  if (modelData?.quota_type === 0) {
    return t('按量计费');
  }
  if (modelData?.quota_type === 1) {
    return t(
      isModelPriceUnitSecond(modelData.model_price_unit)
        ? '按秒计费'
        : '按次计费',
    );
  }
  return t('未知计费类型');
};

const OverviewMetric = ({ icon, label, value, valueStyle }) => {
  const Icon = icon;

  return (
    <div className='classic-pricing-detail-overview-metric'>
      <Icon
        aria-hidden='true'
        className='classic-pricing-detail-overview-metric-icon'
        size={14}
      />
      <div className='classic-pricing-detail-overview-metric-content'>
        <span className='classic-pricing-detail-overview-metric-label'>
          {label}
        </span>
        <strong
          className='classic-pricing-detail-overview-metric-value'
          style={valueStyle}
        >
          {value}
        </strong>
      </div>
    </div>
  );
};

const PillList = ({ items }) => (
  <div className='classic-pricing-detail-pill-list'>
    {items.map((item) => (
      <span key={item} className='classic-pricing-detail-pill'>
        {item}
      </span>
    ))}
  </div>
);

const GroupPillList = ({ groups, groupNames }) => (
  <div className='classic-pricing-detail-pill-list'>
    {groups.map((group) => (
      <span key={group} className='classic-pricing-detail-pill'>
        {getGroupDisplayName(group, groupNames)}
      </span>
    ))}
  </div>
);

const ModelBasicInfo = ({
  modelData,
  groupNames = {},
  performance,
  t,
  variant = 'metadata',
}) => {
  if (!modelData) return null;

  if (variant === 'summary') {
    return (
      <div className='classic-pricing-detail-overview-metrics'>
        <OverviewMetric
          icon={Timer}
          label='TPS'
          value={formatThroughput(performance?.avg_tps)}
        />
        <OverviewMetric
          icon={Timer}
          label={t('平均延迟')}
          value={formatLatency(performance?.avg_latency_ms)}
        />
        <OverviewMetric
          icon={HeartPulse}
          label={t('成功率')}
          value={formatSuccessRate(performance?.success_rate)}
          valueStyle={{
            color: getSuccessRateTextColor(performance?.success_rate),
          }}
        />
      </div>
    );
  }

  const groups = Array.isArray(modelData.enable_groups)
    ? modelData.enable_groups.filter(Boolean)
    : [];
  const endpoints = Array.isArray(modelData.supported_endpoint_types)
    ? modelData.supported_endpoint_types.filter(Boolean)
    : [];

  const cells = [
    modelData.vendor_name && {
      key: 'vendor',
      label: t('供应商'),
      value: modelData.vendor_name,
    },
    {
      key: 'billing',
      label: t('计费类型'),
      value: getBillingType(modelData, t),
    },
    groups.length > 0 && {
      key: 'groups',
      label: t('分组'),
      value: <GroupPillList groups={groups} groupNames={groupNames} />,
    },
    endpoints.length > 0 && {
      key: 'endpoints',
      label: t('API端点'),
      value: <PillList items={endpoints} />,
    },
  ].filter(Boolean);

  if (cells.length === 0) return null;

  return (
    <section className='classic-pricing-detail-model-section'>
      <h3 className='classic-pricing-detail-section-title'>{t('模型')}</h3>
      <div className='classic-pricing-detail-info-grid'>
        {cells.map((cell) => (
          <div key={cell.key} className='classic-pricing-detail-info-cell'>
            <span className='classic-pricing-detail-info-label'>
              {cell.label}
            </span>
            <div className='classic-pricing-detail-info-value'>
              {cell.value}
            </div>
          </div>
        ))}
      </div>
    </section>
  );
};

export default ModelBasicInfo;
