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
import { Button, Dropdown } from '@douyinfe/semi-ui';
import { IconMore } from '@douyinfe/semi-icons';

const RedemptionsActions = ({
  selectedKeys,
  setEditingRedemption,
  setShowEdit,
  batchCopyRedemptions,
  batchDeleteRedemptions,
  batchDeleteSelectedRedemptions,
  loading,
  t,
}) => {
  // Add new redemption code
  const handleAddRedemption = () => {
    setEditingRedemption({
      id: undefined,
    });
    setShowEdit(true);
  };

  // Destructive actions that don't depend on the current selection live in
  // an overflow menu instead of sitting permanently in the toolbar.
  const moreMenuItems = [
    {
      node: 'item',
      name: t('清除失效兑换码'),
      type: 'danger',
      onClick: batchDeleteRedemptions,
    },
  ];

  return (
    <div className='flex flex-wrap gap-2 w-full md:w-auto order-2 md:order-1'>
      <Button
        type='primary'
        className='flex-1 md:flex-initial'
        onClick={handleAddRedemption}
        size='small'
      >
        {t('添加兑换码')}
      </Button>

      <Button
        type='tertiary'
        className='flex-1 md:flex-initial'
        onClick={batchCopyRedemptions}
        size='small'
      >
        {t('复制所选兑换码到剪贴板')}
      </Button>

      {selectedKeys.length > 0 && (
        <Button
          type='danger'
          loading={loading}
          className='w-full md:w-auto'
          onClick={batchDeleteSelectedRedemptions}
          size='small'
        >
          {t('删除所选兑换码')} ({selectedKeys.length})
        </Button>
      )}

      <Dropdown trigger='click' position='bottomRight' menu={moreMenuItems}>
        <Button type='tertiary' size='small' icon={<IconMore />} />
      </Dropdown>
    </div>
  );
};

export default RedemptionsActions;
