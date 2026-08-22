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
import { SideSheet } from '@douyinfe/semi-ui';
import FilterModalContent from './components/FilterModalContent';

const PricingFilterModal = ({
  visible,
  onClose,
  sidebarProps,
  isMobile,
  t,
}) => {
  return (
    <SideSheet
      className='classic-pricing-filter-sheet'
      placement='right'
      title={
        <div className='classic-pricing-filter-sheet-title'>
          <span>{t('筛选')}</span>
          <p>{t('按供应商、分组、类型、端点和标签筛选模型。')}</p>
        </div>
      }
      visible={visible}
      onCancel={onClose}
      width={isMobile ? '100%' : 420}
      bodyStyle={{
        padding: 0,
        overflowY: 'auto',
        scrollbarWidth: 'none',
        msOverflowStyle: 'none',
      }}
    >
      <FilterModalContent sidebarProps={sidebarProps} t={t} />
    </SideSheet>
  );
};

export default PricingFilterModal;
