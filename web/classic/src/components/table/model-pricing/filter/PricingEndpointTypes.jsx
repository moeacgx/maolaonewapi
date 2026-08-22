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

/**
 * 端点类型筛选组件
 * @param {string|'all'} filterEndpointType 当前值
 * @param {Function} setFilterEndpointType setter
 * @param {Array} models 模型列表
 * @param {boolean} loading 是否加载中
 * @param {Function} t i18n
 */
const PricingEndpointTypes = ({
  filterEndpointType,
  setFilterEndpointType,
  models = [],
  allModels = [],
  loading = false,
  t,
}) => {
  const endpointTypeLabels = {
    openai: 'Chat',
    'openai-response': 'Response',
    anthropic: 'Anthropic',
    gemini: 'Gemini',
    'jina-rerank': 'Rerank',
    'image-generation': t('图片'),
    embeddings: t('嵌入'),
    'openai-video': t('视频'),
  };

  const getAllEndpointTypes = () => {
    const endpointTypes = new Set();
    (allModels.length > 0 ? allModels : models).forEach((model) => {
      if (
        model.supported_endpoint_types &&
        Array.isArray(model.supported_endpoint_types)
      ) {
        model.supported_endpoint_types.forEach((endpoint) => {
          endpointTypes.add(endpoint);
        });
      }
    });
    return [
      ...Object.keys(endpointTypeLabels),
      ...Array.from(endpointTypes)
        .filter((endpoint) => !endpointTypeLabels[endpoint])
        .sort(),
    ];
  };

  // 计算每个端点类型的模型数量
  const getEndpointTypeCount = (endpointType) => {
    if (endpointType === 'all') {
      return models.length;
    }
    return models.filter(
      (model) =>
        model.supported_endpoint_types &&
        model.supported_endpoint_types.includes(endpointType),
    ).length;
  };

  const getEndpointTypeLabel = (endpointType) => {
    return endpointTypeLabels[endpointType] || endpointType;
  };

  const availableEndpointTypes = getAllEndpointTypes();

  const items = [
    {
      value: 'all',
      label: t('所有类型'),
      tagCount: getEndpointTypeCount('all'),
    },
    ...availableEndpointTypes.map((endpointType) => {
      const count = getEndpointTypeCount(endpointType);
      return {
        value: endpointType,
        label: getEndpointTypeLabel(endpointType),
        tagCount: count,
      };
    }),
  ];

  return (
    <PricingFilterSection
      title={t('端点类型')}
      items={items}
      activeValue={filterEndpointType}
      onChange={setFilterEndpointType}
      loading={loading}
      t={t}
    />
  );
};

export default PricingEndpointTypes;
