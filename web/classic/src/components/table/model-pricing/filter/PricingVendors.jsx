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
import PricingFilterSection from './PricingFilterSection';
import { getLobeHubIcon } from '../../../../helpers';

/**
 * 供应商筛选组件
 * @param {string|'all'} filterVendor 当前值
 * @param {Function} setFilterVendor setter
 * @param {Array} models 模型列表
 * @param {Array} allModels 所有模型列表（用于获取全部供应商）
 * @param {boolean} loading 是否加载中
 * @param {Function} t i18n
 */
const PricingVendors = ({
  filterVendor,
  setFilterVendor,
  models = [],
  allModels = [],
  vendors = [],
  loading = false,
  t,
}) => {
  // 新版按 /api/pricing 的供应商顺序展示；缺失供应商仍作为兜底保留。
  const vendorOptions = React.useMemo(() => {
    const vendorNames = new Set();
    const vendorIcons = new Map();
    let hasUnknownVendor = false;

    (allModels.length > 0 ? allModels : models).forEach((model) => {
      if (model.vendor_name) {
        vendorNames.add(model.vendor_name);
        if (model.vendor_icon && !vendorIcons.has(model.vendor_name)) {
          vendorIcons.set(model.vendor_name, model.vendor_icon);
        }
      } else {
        hasUnknownVendor = true;
      }
    });

    const orderedNames = [];
    const orderedNameSet = new Set();
    vendors.forEach((vendor) => {
      if (vendor?.name && vendorNames.has(vendor.name)) {
        orderedNames.push(vendor.name);
        orderedNameSet.add(vendor.name);
        if (vendor.icon && !vendorIcons.has(vendor.name)) {
          vendorIcons.set(vendor.name, vendor.icon);
        }
      }
    });

    Array.from(vendorNames)
      .filter((vendorName) => !orderedNameSet.has(vendorName))
      .sort((left, right) => left.localeCompare(right))
      .forEach((vendorName) => orderedNames.push(vendorName));

    return { orderedNames, vendorIcons, hasUnknownVendor };
  }, [allModels, models, vendors]);

  // 计算每个供应商的模型数量（基于当前过滤后的 models）
  const getVendorCount = React.useCallback(
    (vendor) => {
      if (vendor === 'all') {
        return models.length;
      }
      if (vendor === 'unknown') {
        return models.filter((model) => !model.vendor_name).length;
      }
      return models.filter((model) => model.vendor_name === vendor).length;
    },
    [models],
  );

  // 生成供应商选项
  const items = React.useMemo(() => {
    const result = [
      {
        value: 'all',
        label: t('所有供应商'),
        tagCount: getVendorCount('all'),
      },
    ];

    // 添加所有已知供应商
    vendorOptions.orderedNames.forEach((vendor) => {
      const count = getVendorCount(vendor);
      if (count > 0) {
        const icon = vendorOptions.vendorIcons.get(vendor);
        result.push({
          value: vendor,
          label: vendor,
          icon: icon ? getLobeHubIcon(icon, 16) : null,
          tagCount: count,
        });
      }
    });

    // 如果系统中存在未知供应商，添加"未知供应商"选项
    if (vendorOptions.hasUnknownVendor) {
      const count = getVendorCount('unknown');
      result.push({
        value: 'unknown',
        label: t('未知供应商'),
        tagCount: count,
      });
    }

    return result;
  }, [vendorOptions, getVendorCount, t]);

  return (
    <PricingFilterSection
      title={t('所有供应商')}
      items={items}
      activeValue={filterVendor}
      onChange={setFilterVendor}
      loading={loading}
      t={t}
    />
  );
};

export default PricingVendors;
