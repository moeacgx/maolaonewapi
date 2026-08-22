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
import { Avatar, Button, Toast } from '@douyinfe/semi-ui';
import { Copy } from 'lucide-react';
import {
  copy,
  getLobeHubIcon,
  isModelPriceUnitSecond,
} from '../../../../../helpers';

const CARD_STYLES = {
  container: 'classic-pricing-detail-model-icon',
  icon: 'classic-pricing-detail-model-icon-content',
};

const ModelHeader = ({ modelData, vendorsMap = {}, t }) => {
  const modelName = modelData?.model_name || t('未知模型');
  const vendorName =
    modelData?.vendor_name || vendorsMap[modelData?.vendor_id]?.name || '';
  const description = modelData?.description || modelData?.vendor_description;

  const getBillingType = () => {
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

  const handleCopy = async () => {
    if (await copy(modelName)) {
      Toast.success({ content: t('已复制模型名称') });
    }
  };

  // 获取模型图标（优先模型图标，其次供应商图标）
  const getModelIcon = () => {
    // 1) 优先使用模型自定义图标
    if (modelData?.icon) {
      return (
        <div className={CARD_STYLES.container}>
          <div className={CARD_STYLES.icon}>
            {getLobeHubIcon(modelData.icon, 24)}
          </div>
        </div>
      );
    }
    // 2) 退化为供应商图标
    if (modelData?.vendor_icon) {
      return (
        <div className={CARD_STYLES.container}>
          <div className={CARD_STYLES.icon}>
            {getLobeHubIcon(modelData.vendor_icon, 24)}
          </div>
        </div>
      );
    }

    // 如果没有供应商图标，使用模型名称的前两个字符
    const avatarText = modelData?.model_name?.slice(0, 2).toUpperCase() || 'AI';
    return (
      <div className={CARD_STYLES.container}>
        <Avatar
          size='small'
          style={{
            width: 24,
            height: 24,
            borderRadius: 6,
            fontSize: 10,
            fontWeight: 'bold',
          }}
        >
          {avatarText}
        </Avatar>
      </div>
    );
  };

  return (
    <header className='classic-pricing-detail-model-header'>
      <div className='classic-pricing-detail-model-title-row'>
        {getModelIcon()}
        <h1 className='classic-pricing-detail-model-title'>{modelName}</h1>
        <Button
          aria-label={t('点击复制模型名称')}
          className='classic-pricing-detail-copy-button'
          icon={<Copy size={14} />}
          size='small'
          theme='borderless'
          title={t('点击复制模型名称')}
          type='tertiary'
          onClick={handleCopy}
        />
      </div>
      <div className='classic-pricing-detail-model-meta'>
        {vendorName && (
          <span className='classic-pricing-detail-model-vendor'>
            {vendorName}
          </span>
        )}
        {vendorName && (
          <span aria-hidden='true' className='classic-pricing-detail-meta-dot'>
            ·
          </span>
        )}
        <span className='classic-pricing-detail-billing-badge'>
          {getBillingType()}
        </span>
      </div>
      {description && (
        <p className='classic-pricing-detail-model-description'>
          {description}
        </p>
      )}
    </header>
  );
};

export default ModelHeader;
