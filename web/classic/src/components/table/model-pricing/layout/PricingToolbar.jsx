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
import { Button, Dropdown, Select } from '@douyinfe/semi-ui';
import {
  ArrowUpDown,
  Copy,
  Filter,
  Gauge,
  Grid2X2,
  HelpCircle,
  ListChecks,
  MoreHorizontal,
  Table2,
} from 'lucide-react';

const PricingSegmentedControl = ({
  ariaLabel,
  options,
  value,
  onChange,
  className = '',
}) => (
  <div
    className={`classic-pricing-segmented-control ${className}`}
    role='group'
    aria-label={ariaLabel}
  >
    {options.map((option) => {
      const active = option.value === value;
      const Icon = option.icon;
      return (
        <button
          key={option.value}
          type='button'
          className={active ? 'is-active' : ''}
          aria-pressed={active}
          title={option.title || option.label}
          onClick={() => onChange(option.value)}
        >
          {Icon && <Icon size={15} aria-hidden='true' />}
          {option.label && <span>{option.label}</span>}
        </button>
      );
    })}
  </div>
);

const PricingToolbar = ({
  models = [],
  filteredModels = [],
  selectedRowKeys = [],
  setSelectedRowKeys,
  selectionMode = false,
  setSelectionMode,
  copyText,
  onOpenFilters,
  isMobile,
  searchValue,
  filterGroup,
  filterQuotaType,
  filterEndpointType,
  filterVendor,
  filterTag,
  showWithRecharge,
  setShowWithRecharge,
  currency,
  setCurrency,
  siteDisplayType,
  showRatio,
  setShowRatio,
  viewMode,
  setViewMode,
  tokenUnit,
  setTokenUnit,
  sortBy = 'name',
  setSortBy,
  onOpenBillingGuide,
  t,
}) => {
  const supportsCurrencyDisplay = siteDisplayType !== 'TOKENS';
  const activeFilterCount = [
    filterGroup,
    filterQuotaType,
    filterEndpointType,
    filterVendor,
    filterTag,
  ].filter((filterValue) => filterValue && filterValue !== 'all').length;
  const hasActiveFilters = activeFilterCount > 0 || Boolean(searchValue);
  const sortOptions = [
    { value: 'name', label: t('名称') },
    { value: 'price-low', label: t('价格从低到高') },
    { value: 'price-high', label: t('价格从高到低') },
  ];
  const sortLabel =
    sortOptions.find((option) => option.value === sortBy)?.label || t('名称');
  const sortMenu = sortOptions.map((option) => ({
    node: 'item',
    name: option.label,
    selected: sortBy === option.value,
    onClick: () => setSortBy?.(option.value),
  }));

  const handleCopy = React.useCallback(() => {
    if (selectedRowKeys.length > 0) {
      copyText?.(selectedRowKeys.join('\n'));
    }
  }, [copyText, selectedRowKeys]);

  const handleSelectionModeChange = React.useCallback(() => {
    const nextSelectionMode = !selectionMode;
    setSelectionMode?.(nextSelectionMode);
    if (!nextSelectionMode) {
      setSelectedRowKeys?.([]);
    }
  }, [selectionMode, setSelectedRowKeys, setSelectionMode]);

  return (
    <div className='classic-pricing-toolbar'>
      <div className='classic-pricing-toolbar-summary' aria-live='polite'>
        <Button
          theme='outline'
          type='tertiary'
          icon={<Filter size={16} />}
          onClick={onOpenFilters}
          className='classic-pricing-toolbar-filter-button'
        >
          {t('筛选')}
          {activeFilterCount > 0 && (
            <span className='classic-pricing-toolbar-filter-count'>
              {activeFilterCount}
            </span>
          )}
        </Button>
        <div className='classic-pricing-toolbar-count'>
          <strong>{filteredModels.length}</strong>
          <span>{t('个模型')}</span>
          {hasActiveFilters && <span>/ {models.length}</span>}
        </div>
      </div>

      <div className='classic-pricing-toolbar-controls'>
        {supportsCurrencyDisplay && (
          <PricingSegmentedControl
            ariaLabel={t('价格显示模式')}
            className='classic-pricing-toolbar-price-display'
            value={showWithRecharge ? 'recharge' : 'standard'}
            onChange={(value) => setShowWithRecharge?.(value === 'recharge')}
            options={[
              { value: 'standard', label: t('标准') },
              { value: 'recharge', label: t('充值') },
            ]}
          />
        )}

        {supportsCurrencyDisplay && showWithRecharge && (
          <Select
            value={currency}
            onChange={setCurrency}
            aria-label={t('货币单位')}
            className='classic-pricing-toolbar-currency'
            optionList={[
              { value: 'USD', label: 'USD' },
              { value: 'CNY', label: 'CNY' },
              { value: 'CUSTOM', label: t('自定义货币') },
            ]}
          />
        )}

        <PricingSegmentedControl
          ariaLabel={t('Token 单位')}
          className='classic-pricing-toolbar-token-unit'
          value={tokenUnit}
          onChange={(value) => setTokenUnit?.(value)}
          options={[
            { value: 'M', label: '/1M' },
            { value: 'K', label: '/1K' },
          ]}
        />

        <Dropdown
          position='bottomRight'
          showTick
          menu={sortMenu}
          trigger='click'
        >
          <Button
            theme='outline'
            type='tertiary'
            icon={<ArrowUpDown size={14} />}
            aria-label={t('排序')}
            className='classic-pricing-toolbar-sort-trigger'
          >
            {sortLabel}
          </Button>
        </Dropdown>

        <PricingSegmentedControl
          ariaLabel={t('视图模式')}
          className='classic-pricing-toolbar-view-switcher'
          value={viewMode}
          onChange={(value) => setViewMode?.(value)}
          options={[
            {
              value: 'card',
              icon: Grid2X2,
              title: t('卡片视图'),
            },
            {
              value: 'table',
              icon: Table2,
              title: t('表格视图'),
            },
          ]}
        />

        <Dropdown
          trigger='click'
          position='bottomRight'
          clickToHide
          render={
            <Dropdown.Menu>
              <Dropdown.Item
                icon={<ListChecks size={15} />}
                onClick={handleSelectionModeChange}
              >
                {selectionMode ? t('退出批量选择') : t('批量选择')}
              </Dropdown.Item>
              <Dropdown.Item
                icon={<Gauge size={15} />}
                onClick={() => setShowRatio?.(!showRatio)}
              >
                {t('倍率')}
              </Dropdown.Item>
              {isMobile && supportsCurrencyDisplay && (
                <Dropdown.Item
                  onClick={() => setShowWithRecharge?.(!showWithRecharge)}
                >
                  {showWithRecharge ? t('标准') : t('充值')}
                </Dropdown.Item>
              )}
              {isMobile && supportsCurrencyDisplay && showWithRecharge && (
                <>
                  <Dropdown.Item onClick={() => setCurrency?.('USD')}>
                    USD
                  </Dropdown.Item>
                  <Dropdown.Item onClick={() => setCurrency?.('CNY')}>
                    CNY
                  </Dropdown.Item>
                  <Dropdown.Item onClick={() => setCurrency?.('CUSTOM')}>
                    {t('自定义货币')}
                  </Dropdown.Item>
                </>
              )}
              {isMobile && (
                <Dropdown.Item
                  onClick={() => setTokenUnit?.(tokenUnit === 'M' ? 'K' : 'M')}
                >
                  {tokenUnit === 'M' ? '/1K' : '/1M'}
                </Dropdown.Item>
              )}
              <Dropdown.Item
                icon={<Copy size={15} />}
                disabled={selectedRowKeys.length === 0}
                onClick={handleCopy}
              >
                {t('复制')}
              </Dropdown.Item>
              {onOpenBillingGuide && (
                <Dropdown.Item
                  icon={<HelpCircle size={15} />}
                  onClick={onOpenBillingGuide}
                >
                  {t('计费说明')}
                </Dropdown.Item>
              )}
            </Dropdown.Menu>
          }
        >
          <Button
            theme='borderless'
            type='tertiary'
            icon={<MoreHorizontal size={16} />}
            aria-label={t('更多操作')}
            title={t('更多操作')}
            className='classic-pricing-toolbar-icon-button'
          />
        </Dropdown>
      </div>
    </div>
  );
};

export default React.memo(PricingToolbar);
