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
  Card,
  Tooltip,
  Checkbox,
  Button,
  Avatar,
} from '@douyinfe/semi-ui';
import {
  IconChevronLeft,
  IconChevronRight,
  IconHelpCircle,
} from '@douyinfe/semi-icons';
import { Copy, Search } from 'lucide-react';
import {
  calculateModelPrice,
  getModelPriceItems,
  getLobeHubIcon,
  isModelPriceUnitSecond,
} from '../../../../../helpers';
import PricingCardSkeleton from './PricingCardSkeleton';
import ModelPerformanceBadge from './ModelPerformanceBadge';
import { useMinimumLoadingTime } from '../../../../../hooks/common/useMinimumLoadingTime';
import { useIsMobile } from '../../../../../hooks/common/useIsMobile';

const CARD_STYLES = {
  container:
    'classic-pricing-model-card-icon w-10 h-10 rounded-lg flex items-center justify-center relative shrink-0',
  icon: 'w-7 h-7 flex items-center justify-center',
  selected: 'is-selected',
  default: '',
};

const COMPACT_PRICE_LABELS = {
  input: '输入',
  completion: '输出',
  cache: '缓存',
  'create-cache': '缓存创建',
  image: '图片',
  'audio-input': '音频输入',
  'audio-output': '音频输出',
  'input-ratio': '输入',
  'completion-ratio': '输出',
  'cache-ratio': '缓存',
  'create-cache-ratio': '缓存创建',
  'image-ratio': '图片',
  'audio-input-ratio': '音频输入',
  'audio-output-ratio': '音频输出',
};

const formatCompactPrice = (value) =>
  String(value)
    .replace(/(\.\d*?[1-9])0+$/u, '$1')
    .replace(/\.0+$/u, '');

const renderCompactPriceSummary = (priceData, t, siteDisplayType) => {
  if (priceData.isDynamicPricing) {
    return (
      <span className='classic-pricing-model-card-price-item'>
        <span className='classic-pricing-model-card-price-label'>
          {t('动态计费')}
        </span>
      </span>
    );
  }

  const items = getModelPriceItems(priceData, t, siteDisplayType);
  const visibleItems = priceData.isPerToken
    ? items.slice(0, 3)
    : items.slice(0, 1);

  return visibleItems.map((item) => {
    const label = COMPACT_PRICE_LABELS[item.key];
    const value = item.isVariantRange ? item.minimumValue : item.value;

    return (
      <span key={item.key} className='classic-pricing-model-card-price-item'>
        {label && (
          <span className='classic-pricing-model-card-price-label'>
            {t(label)}
          </span>
        )}
        <span className='classic-pricing-model-card-price-value'>
          {formatCompactPrice(value)}
        </span>
        {!priceData.isPerToken && item.suffix && (
          <span className='classic-pricing-model-card-price-suffix'>
            {item.suffix}
          </span>
        )}
      </span>
    );
  });
};

const PricingCardView = ({
  filteredModels,
  loading,
  rowSelection,
  pageSize,
  setPageSize,
  currentPage,
  setCurrentPage,
  selectedGroup,
  groupRatio,
  copyText,
  setModalImageUrl,
  setIsModalOpenurl,
  currency,
  siteDisplayType,
  tokenUnit,
  displayPrice,
  showRatio,
  t,
  selectedRowKeys = [],
  setSelectedRowKeys,
  selectionMode = false,
  openModelDetail,
  performanceMap = {},
}) => {
  const DEFAULT_CARD_PAGE_SIZE = 20;
  const showSkeleton = useMinimumLoadingTime(loading);
  const totalPages = Math.max(
    1,
    Math.ceil((filteredModels?.length || 0) / DEFAULT_CARD_PAGE_SIZE),
  );
  const displayPage = Math.min(currentPage, totalPages);
  const startIndex = (displayPage - 1) * DEFAULT_CARD_PAGE_SIZE;
  const paginatedModels = filteredModels.slice(
    startIndex,
    startIndex + DEFAULT_CARD_PAGE_SIZE,
  );
  const getModelKey = (model) => model.key ?? model.model_name ?? model.id;
  const isMobile = useIsMobile();

  React.useEffect(() => {
    if (pageSize !== DEFAULT_CARD_PAGE_SIZE) {
      setPageSize(DEFAULT_CARD_PAGE_SIZE);
    }
    if (currentPage !== displayPage) {
      setCurrentPage(displayPage);
    }
  }, [currentPage, displayPage, pageSize, setCurrentPage, setPageSize]);

  const handleCheckboxChange = (model, checked) => {
    if (!setSelectedRowKeys) return;
    const modelKey = getModelKey(model);
    const newKeys = checked
      ? Array.from(new Set([...selectedRowKeys, modelKey]))
      : selectedRowKeys.filter((key) => key !== modelKey);
    setSelectedRowKeys(newKeys);
    rowSelection?.onChange?.(newKeys, null);
  };

  // 获取模型图标
  const getModelIcon = (model) => {
    if (!model || !model.model_name) {
      return (
        <div className={CARD_STYLES.container}>
          <Avatar size='large'>?</Avatar>
        </div>
      );
    }
    // 1) 优先使用模型自定义图标
    if (model.icon) {
      return (
        <div className={CARD_STYLES.container}>
          <div className={CARD_STYLES.icon}>
            {getLobeHubIcon(model.icon, 32)}
          </div>
        </div>
      );
    }
    // 2) 退化为供应商图标
    if (model.vendor_icon) {
      return (
        <div className={CARD_STYLES.container}>
          <div className={CARD_STYLES.icon}>
            {getLobeHubIcon(model.vendor_icon, 32)}
          </div>
        </div>
      );
    }

    // 如果没有供应商图标，使用模型名称生成头像

    const avatarText = model.model_name.slice(0, 2).toUpperCase();
    return (
      <div className={CARD_STYLES.container}>
        <Avatar
          size='large'
          style={{
            width: 40,
            height: 40,
            borderRadius: 8,
            fontSize: 16,
            fontWeight: 'bold',
          }}
        >
          {avatarText}
        </Avatar>
      </div>
    );
  };

  const renderBillingTag = (record) => {
    let billingMode = 'classic-pricing-billing-mode-neutral';
    let label = '-';

    if (record.quota_type === 1) {
      label = t(
        isModelPriceUnitSecond(record.model_price_unit)
          ? '按秒计费'
          : '按次计费',
      );
      billingMode = isModelPriceUnitSecond(record.model_price_unit)
        ? 'classic-pricing-billing-mode-neutral'
        : 'classic-pricing-billing-mode-purple';
    } else if (record.quota_type === 0) {
      label = t('按量计费');
      billingMode = 'classic-pricing-billing-mode-info';
    }

    if (record.billing_mode === 'tiered_expr') {
      label = t('动态计费');
      billingMode = 'classic-pricing-billing-mode-warning';
    }

    return (
      <span className={`classic-pricing-billing-mode ${billingMode}`}>
        {label}
      </span>
    );
  };

  // 显示骨架屏
  if (showSkeleton) {
    return (
      <PricingCardSkeleton
        rowSelection={selectionMode && !!rowSelection}
        showRatio={showRatio}
        isMobile={isMobile}
      />
    );
  }

  if (!filteredModels || filteredModels.length === 0) {
    return (
      <div className='classic-pricing-empty-state'>
        <Search size={40} aria-hidden='true' />
        <h3>{t('搜索无结果')}</h3>
      </div>
    );
  }

  return (
    <div className='classic-pricing-card-list'>
      <div className='classic-pricing-card-grid'>
        {paginatedModels.map((model, index) => {
          const modelKey = getModelKey(model);
          const isSelected = selectedRowKeys.includes(modelKey);

          const priceData = calculateModelPrice({
            record: model,
            selectedGroup,
            groupRatio,
            tokenUnit,
            displayPrice,
            currency,
            quotaDisplayType: siteDisplayType,
          });

          return (
            <Card
              key={modelKey || index}
              className={`classic-pricing-model-card ${
                isSelected ? CARD_STYLES.selected : CARD_STYLES.default
              }`}
              bodyStyle={{
                padding: isMobile ? 14 : 16,
              }}
            >
              <div className='classic-pricing-model-card-body'>
                <div className='classic-pricing-model-card-header'>
                  <div className='classic-pricing-model-card-title-wrap'>
                    {getModelIcon(model)}
                    <div className='classic-pricing-model-card-title-content'>
                      <div className='classic-pricing-model-card-title-row'>
                        <h3 className='classic-pricing-model-card-title'>
                          {model.model_name}
                        </h3>
                      </div>
                      <div className='classic-pricing-model-card-prices'>
                        {renderCompactPriceSummary(
                          priceData,
                          t,
                          siteDisplayType,
                        )}
                      </div>
                    </div>
                  </div>

                  <div className='classic-pricing-model-card-actions'>
                    <Button
                      size='small'
                      theme='borderless'
                      type='tertiary'
                      onClick={(e) => {
                        e.stopPropagation();
                        openModelDetail?.(model);
                      }}
                      aria-label={t('详情')}
                      className='classic-pricing-model-card-detail-button'
                    >
                      <span>{t('详情')}</span>
                      <IconChevronRight size='small' aria-hidden='true' />
                    </Button>

                    <Button
                      size='small'
                      theme='borderless'
                      type='tertiary'
                      icon={<Copy size={12} />}
                      onClick={(e) => {
                        e.stopPropagation();
                        copyText(model.model_name);
                      }}
                      aria-label={t('复制')}
                      title={t('复制')}
                      className='classic-pricing-model-card-copy-button'
                    />

                    {selectionMode && rowSelection && (
                      <Checkbox
                        checked={isSelected}
                        onChange={(e) => {
                          e.stopPropagation();
                          handleCheckboxChange(model, e.target.checked);
                        }}
                      />
                    )}
                  </div>
                </div>

                <p className='classic-pricing-model-card-description'>
                  {model.description || t('暂无描述。')}
                </p>

                <div className='classic-pricing-model-card-footer'>
                  <div className='classic-pricing-model-card-footer-info'>
                    <div className='classic-pricing-model-card-billing'>
                      {renderBillingTag(model)}
                    </div>
                    <ModelPerformanceBadge
                      performance={performanceMap[model.model_name]}
                      t={t}
                      isMobile={isMobile}
                    />
                  </div>

                  {showRatio && (
                    <div className='classic-pricing-model-card-ratios'>
                      <div className='classic-pricing-model-card-ratio-title'>
                        <span>{t('倍率信息')}</span>
                        <Tooltip
                          content={t('倍率是为了方便换算不同价格的模型')}
                        >
                          <IconHelpCircle
                            className='cursor-pointer'
                            size='small'
                            onClick={(e) => {
                              e.stopPropagation();
                              setModalImageUrl('/ratio.png');
                              setIsModalOpenurl(true);
                            }}
                          />
                        </Tooltip>
                      </div>
                      <div className='classic-pricing-model-card-ratio-grid'>
                        <div>
                          <span>{t('模型')}</span>
                          <strong>
                            {model.quota_type === 0
                              ? model.model_ratio
                              : t('无')}
                          </strong>
                        </div>
                        <div>
                          <span>{t('补全')}</span>
                          <strong>
                            {model.quota_type === 0
                              ? parseFloat(model.completion_ratio.toFixed(3))
                              : t('无')}
                          </strong>
                        </div>
                        <div>
                          <span>{t('分组')}</span>
                          <strong>{priceData?.usedGroupRatio ?? '-'}</strong>
                        </div>
                      </div>
                    </div>
                  )}
                </div>
              </div>
            </Card>
          );
        })}
      </div>

      {totalPages > 1 && (
        <div className='classic-pricing-pagination'>
          <span className='classic-pricing-pagination-count'>
            {displayPage} / {totalPages}
          </span>
          <div className='classic-pricing-pagination-actions'>
            <Button
              type='tertiary'
              theme='outline'
              size='small'
              icon={<IconChevronLeft aria-hidden='true' />}
              disabled={displayPage <= 1}
              onClick={() => setCurrentPage(displayPage - 1)}
              aria-label={t('上一步')}
            >
              {t('上一步')}
            </Button>
            <Button
              type='tertiary'
              theme='outline'
              size='small'
              icon={<IconChevronRight aria-hidden='true' />}
              disabled={displayPage >= totalPages}
              onClick={() => setCurrentPage(displayPage + 1)}
              aria-label={t('下一步')}
            >
              {t('下一步')}
            </Button>
          </div>
        </div>
      )}
    </div>
  );
};

export default PricingCardView;
