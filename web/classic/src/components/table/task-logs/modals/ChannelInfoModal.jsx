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
import { Modal, Badge, Tag, Typography } from '@douyinfe/semi-ui';
import { CHANNEL_OPTIONS } from '../../../../constants/channel.constants';
import { renderQuota, renderQuotaWithAmount } from '../../../../helpers';

const { Text } = Typography;

const ChannelInfoModal = ({
  showChannelInfo,
  setShowChannelInfoModal,
  channelInfoData,
  t,
}) => {
  const infoItemStyle = {
    marginBottom: '16px',
  };

  const labelStyle = {
    display: 'flex',
    alignItems: 'center',
    marginBottom: '2px',
    fontSize: '12px',
    color: 'var(--semi-color-text-2)',
    gap: '6px',
  };

  const renderLabel = (text, type = 'tertiary') => (
    <div style={labelStyle}>
      <Badge dot type={type} />
      {text}
    </div>
  );

  const valueStyle = {
    fontSize: '14px',
    fontWeight: '600',
    color: 'var(--semi-color-text-0)',
    wordBreak: 'break-all',
  };

  const rowStyle = {
    display: 'flex',
    justifyContent: 'space-between',
    marginBottom: '16px',
    gap: '20px',
  };

  const colStyle = {
    flex: 1,
    minWidth: 0,
  };

  const channelTypeName =
    CHANNEL_OPTIONS.find((option) => option.value === channelInfoData?.type)
      ?.label || channelInfoData?.type;
  const groups = Array.isArray(channelInfoData?.groups)
    ? channelInfoData.groups
    : String(channelInfoData?.group || '')
        .split(',')
        .map((item) => item.trim())
        .filter(Boolean);
  const models = Array.isArray(channelInfoData?.models)
    ? channelInfoData.models
    : String(channelInfoData?.models || '')
        .split(',')
        .map((item) => item.trim())
        .filter(Boolean);

  return (
    <Modal
      title={t('渠道信息')}
      visible={showChannelInfo}
      onCancel={() => setShowChannelInfoModal(false)}
      footer={null}
      centered
      closable
      maskClosable
      width={680}
    >
      {channelInfoData && (
        <div style={{ padding: 20 }}>
          <div style={rowStyle}>
            <div style={colStyle}>
              {renderLabel('ID', 'primary')}
              <div style={valueStyle}>#{channelInfoData.id}</div>
            </div>
            <div style={colStyle}>
              {renderLabel(t('名称'), 'primary')}
              <div style={valueStyle}>{channelInfoData.name || '-'}</div>
            </div>
          </div>

          <div style={rowStyle}>
            <div style={colStyle}>
              {renderLabel(t('类型'), 'tertiary')}
              <div style={valueStyle}>{channelTypeName || '-'}</div>
            </div>
            <div style={colStyle}>
              {renderLabel(t('状态'), 'success')}
              <div style={valueStyle}>{channelInfoData.status ?? '-'}</div>
            </div>
          </div>

          <div style={rowStyle}>
            <div style={colStyle}>
              {renderLabel(t('余额'), 'success')}
              <div style={valueStyle}>
                {renderQuotaWithAmount(channelInfoData.balance)}
              </div>
            </div>
            <div style={colStyle}>
              {renderLabel(t('已用额度'), 'warning')}
              <div style={valueStyle}>
                {renderQuota(channelInfoData.used_quota)}
              </div>
            </div>
          </div>

          <div style={infoItemStyle}>
            {renderLabel(t('分组'), 'tertiary')}
            <div className='flex flex-wrap gap-2'>
              {groups.length > 0 ? (
                groups.map((group) => (
                  <Tag key={group} color='blue' shape='circle'>
                    {group}
                  </Tag>
                ))
              ) : (
                <Text type='tertiary'>-</Text>
              )}
            </div>
          </div>

          <div style={{ marginBottom: 0 }}>
            {renderLabel(t('模型'), 'tertiary')}
            <div className='max-h-40 overflow-y-auto rounded border border-[var(--semi-color-border)] p-2'>
              {models.length > 0 ? (
                <div className='flex flex-wrap gap-2'>
                  {models.map((model) => (
                    <Tag key={model} color='purple' shape='circle'>
                      {model}
                    </Tag>
                  ))}
                </div>
              ) : (
                <Text type='tertiary'>-</Text>
              )}
            </div>
          </div>
        </div>
      )}
    </Modal>
  );
};

export default ChannelInfoModal;
