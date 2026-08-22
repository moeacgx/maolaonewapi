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

import React, { useLayoutEffect, useState } from 'react';
import { SideSheet, Typography, Tabs, TabPane } from '@douyinfe/semi-ui';
import { Code2, HeartPulse, Info, X } from 'lucide-react';

import { useIsMobile } from '../../../../hooks/common/useIsMobile';
import ModelHeader from './components/ModelHeader';
import ModelBasicInfo from './components/ModelBasicInfo';
import ModelEndpoints from './components/ModelEndpoints';
import ModelPricingTable from './components/ModelPricingTable';
import ModelPerformancePanel from '../performance/ModelPerformancePanel';

const { Text } = Typography;

const ModelDetailSideSheet = ({
  visible,
  onClose,
  modelData,
  groupRatio,
  groupNames,
  currency,
  siteDisplayType,
  tokenUnit,
  displayPrice,
  priceRate,
  usdExchangeRate,
  usableGroup,
  vendorsMap,
  endpointMap,
  autoGroups,
  performance,
  t,
}) => {
  const isMobile = useIsMobile();
  const [activeTab, setActiveTab] = useState('overview');

  useLayoutEffect(() => {
    setActiveTab('overview');
  }, [modelData?.model_name, visible]);

  return (
    <SideSheet
      className='classic-pricing-detail-sheet'
      placement='right'
      aria-label={t('模型详情')}
      bodyStyle={{ padding: '0' }}
      visible={visible}
      width={isMobile ? '100%' : 'min(1024px, 70vw, calc(100vw - 32px))'}
      closeIcon={<X size={18} />}
      onCancel={onClose}
    >
      <div className='classic-pricing-detail-shell'>
        {!modelData && (
          <div className='classic-pricing-detail-loading'>
            <Text type='secondary'>{t('加载中...')}</Text>
          </div>
        )}
        {modelData && (
          <div className='classic-pricing-detail-content'>
            <ModelHeader modelData={modelData} vendorsMap={vendorsMap} t={t} />
            <div className='classic-pricing-detail-tabs'>
              <Tabs
                type='button'
                activeKey={activeTab}
                onChange={setActiveTab}
                keepDOM
                lazyRender
                collapsible={false}
                tabBarStyle={{
                  display: 'grid',
                  gridTemplateColumns: 'repeat(3, minmax(0, 1fr))',
                  width: '100%',
                }}
              >
                <TabPane
                  tab={
                    <span className='classic-pricing-detail-tab-label'>
                      <Info size={14} />
                      {t('概览')}
                    </span>
                  }
                  itemKey='overview'
                >
                  <div className='classic-pricing-detail-tab-panel'>
                    <section className='classic-pricing-detail-section'>
                      <ModelBasicInfo
                        modelData={modelData}
                        performance={performance}
                        t={t}
                        variant='summary'
                      />
                    </section>
                    <section className='classic-pricing-detail-section'>
                      <ModelPricingTable
                        modelData={modelData}
                        groupRatio={groupRatio}
                        groupNames={groupNames}
                        currency={currency}
                        siteDisplayType={siteDisplayType}
                        tokenUnit={tokenUnit}
                        displayPrice={displayPrice}
                        priceRate={priceRate}
                        usdExchangeRate={usdExchangeRate}
                        usableGroup={usableGroup}
                        autoGroups={autoGroups}
                        t={t}
                      />
                    </section>
                    <section className='classic-pricing-detail-section'>
                      <ModelBasicInfo
                        groupNames={groupNames}
                        modelData={modelData}
                        t={t}
                      />
                    </section>
                  </div>
                </TabPane>
                <TabPane
                  tab={
                    <span className='classic-pricing-detail-tab-label'>
                      <HeartPulse size={14} />
                      {t('性能')}
                    </span>
                  }
                  itemKey='performance'
                >
                  <div className='classic-pricing-detail-tab-panel'>
                    <ModelPerformancePanel
                      modelName={modelData.model_name}
                      groupNames={groupNames}
                      t={t}
                    />
                  </div>
                </TabPane>
                <TabPane
                  tab={
                    <span className='classic-pricing-detail-tab-label'>
                      <Code2 size={14} />
                      API
                    </span>
                  }
                  itemKey='api'
                >
                  <div className='classic-pricing-detail-tab-panel'>
                    <ModelEndpoints
                      modelData={modelData}
                      endpointMap={endpointMap}
                      t={t}
                    />
                  </div>
                </TabPane>
              </Tabs>
            </div>
          </div>
        )}
      </div>
    </SideSheet>
  );
};

export default ModelDetailSideSheet;
