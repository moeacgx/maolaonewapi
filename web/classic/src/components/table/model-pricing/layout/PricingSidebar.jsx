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
import { Button } from '@douyinfe/semi-ui';
import { IconRefresh } from '@douyinfe/semi-icons';
import PricingFilterSections from '../filter/PricingFilterSections';

const PricingSidebar = ({
  filterGroup,
  setFilterGroup,
  setSelectedGroup,
  handleGroupClick,
  filterQuotaType,
  setFilterQuotaType,
  filterEndpointType,
  setFilterEndpointType,
  filterVendor,
  setFilterVendor,
  filterTag,
  setFilterTag,
  setCurrentPage,
  loading,
  t,
  ...categoryProps
}) => {
  const hasActiveFilters = [
    filterGroup,
    filterQuotaType,
    filterEndpointType,
    filterVendor,
    filterTag,
  ].some((value) => value !== undefined && value !== null && value !== 'all');

  const handleResetFilters = () => {
    setFilterGroup('all');
    setSelectedGroup('all');
    setFilterQuotaType('all');
    setFilterEndpointType('all');
    setFilterVendor('all');
    setFilterTag('all');
    setCurrentPage(1);
  };

  return (
    <aside className='classic-pricing-filter-panel'>
      <div className='classic-pricing-filter-panel-header'>
        <div className='min-w-0'>
          <h2 className='classic-pricing-filter-panel-title'>{t('筛选')}</h2>
          <div className='classic-pricing-filter-panel-count'>
            {t('按供应商、分组、类型和标签细化模型。')}
          </div>
        </div>
        <div className='classic-pricing-filter-panel-actions'>
          {hasActiveFilters && (
            <span className='classic-pricing-filter-active-indicator'>
              {t('筛选')}
            </span>
          )}
          <Button
            theme='borderless'
            type='tertiary'
            onClick={handleResetFilters}
            icon={<IconRefresh />}
            disabled={!hasActiveFilters}
            className='classic-pricing-reset-button'
          >
            {t('重置')}
          </Button>
        </div>
      </div>

      <PricingFilterSections
        {...categoryProps}
        filterGroup={filterGroup}
        setFilterGroup={setFilterGroup}
        handleGroupClick={handleGroupClick}
        filterQuotaType={filterQuotaType}
        setFilterQuotaType={setFilterQuotaType}
        filterEndpointType={filterEndpointType}
        setFilterEndpointType={setFilterEndpointType}
        filterVendor={filterVendor}
        setFilterVendor={setFilterVendor}
        filterTag={filterTag}
        setFilterTag={setFilterTag}
        loading={loading}
        t={t}
      />
    </aside>
  );
};

export default PricingSidebar;
