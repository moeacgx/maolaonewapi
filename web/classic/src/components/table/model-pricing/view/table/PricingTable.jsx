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

import React, { useMemo } from 'react';
import { Table } from '@douyinfe/semi-ui';
import { Search } from 'lucide-react';
import { getPricingTableColumns } from './PricingTableColumns';
import {
  getActiveRowSelection,
  getVisiblePricingColumns,
} from './table-view-options';

const PricingTable = ({
  filteredModels,
  loading,
  rowSelection,
  pageSize,
  setPageSize,
  selectedGroup,
  groupRatio,
  groupNames,
  copyText,
  setModalImageUrl,
  setIsModalOpenurl,
  currency,
  siteDisplayType,
  tokenUnit,
  displayPrice,
  searchValue,
  showRatio,
  compactMode = false,
  selectionMode = false,
  openModelDetail,
  t,
}) => {
  const columns = useMemo(() => {
    return getPricingTableColumns({
      t,
      selectedGroup,
      groupRatio,
      groupNames,
      copyText,
      setModalImageUrl,
      setIsModalOpenurl,
      currency,
      siteDisplayType,
      tokenUnit,
      displayPrice,
      showRatio,
    });
  }, [
    t,
    selectedGroup,
    groupRatio,
    groupNames,
    copyText,
    setModalImageUrl,
    setIsModalOpenurl,
    currency,
    siteDisplayType,
    tokenUnit,
    displayPrice,
    showRatio,
  ]);

  // 更新列定义中的 searchValue
  const processedColumns = useMemo(() => {
    const cols = columns.map((column) => {
      if (column.dataIndex === 'model_name') {
        return {
          ...column,
          filteredValue: searchValue ? [searchValue] : [],
        };
      }
      return column;
    });

    return getVisiblePricingColumns(cols, compactMode);
  }, [columns, searchValue, compactMode]);

  const activeRowSelection = useMemo(
    () => getActiveRowSelection(selectionMode, rowSelection),
    [rowSelection, selectionMode],
  );

  const ModelTable = useMemo(
    () => (
      <div className='classic-pricing-table-shell'>
        <Table
          className='classic-pricing-table'
          columns={processedColumns}
          dataSource={filteredModels}
          loading={loading}
          rowSelection={activeRowSelection}
          scroll={compactMode ? undefined : { x: 'max-content' }}
          onRow={(record) => ({
            onClick: () => openModelDetail && openModelDetail(record),
            style: { cursor: 'pointer' },
          })}
          empty={
            <div className='classic-pricing-empty-state'>
              <Search size={40} aria-hidden='true' />
              <h3>{t('搜索无结果')}</h3>
            </div>
          }
          pagination={{
            defaultPageSize: 20,
            pageSize: pageSize,
            showSizeChanger: true,
            pageSizeOptions: [10, 20, 30, 40, 50, 100],
            onPageSizeChange: (size) => setPageSize(size),
          }}
        />
      </div>
    ),
    [
      filteredModels,
      loading,
      processedColumns,
      rowSelection,
      activeRowSelection,
      pageSize,
      setPageSize,
      openModelDetail,
      t,
      compactMode,
    ],
  );

  return ModelTable;
};

export default PricingTable;
