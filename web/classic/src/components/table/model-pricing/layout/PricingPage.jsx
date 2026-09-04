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
import { ImagePreview } from '@douyinfe/semi-ui';
import PricingContent from './content/PricingContent';
import ModelDetailSideSheet from '../modal/ModelDetailSideSheet';
import BillingGuide from '../billing/BillingGuide';
import { getBillingGuideModels } from '../billing/utils';
import { useModelPricingData } from '../../../../hooks/model-pricing/useModelPricingData';
import { useIsMobile } from '../../../../hooks/common/useIsMobile';

const PricingPage = () => {
  const pricingData = useModelPricingData();
  const isMobile = useIsMobile();
  const [showRatio, setShowRatio] = React.useState(false);
  const [viewMode, setViewMode] = React.useState('card');
  const [selectionMode, setSelectionMode] = React.useState(false);
  const [billingGuideVisible, setBillingGuideVisible] = React.useState(false);
  const billingGuideAvailable = React.useMemo(
    () =>
      pricingData.siteDisplayType !== 'TOKENS' &&
      getBillingGuideModels(pricingData.models).length > 0,
    [pricingData.models, pricingData.siteDisplayType],
  );

  const openBillingGuide = React.useCallback(() => {
    setBillingGuideVisible(true);
  }, []);

  React.useEffect(() => {
    if (!billingGuideAvailable) {
      setBillingGuideVisible(false);
    }
  }, [billingGuideAvailable]);

  const allProps = {
    ...pricingData,
    showRatio,
    setShowRatio,
    viewMode,
    setViewMode,
    selectionMode,
    setSelectionMode,
    onOpenBillingGuide: billingGuideAvailable ? openBillingGuide : undefined,
  };

  return (
    <div className='classic-pricing-root'>
      <PricingContent
        {...allProps}
        isMobile={isMobile}
        sidebarProps={allProps}
      />

      <ImagePreview
        src={pricingData.modalImageUrl}
        visible={pricingData.isModalOpenurl}
        onVisibleChange={(visible) => pricingData.setIsModalOpenurl(visible)}
      />

      <ModelDetailSideSheet
        visible={pricingData.showModelDetail}
        onClose={pricingData.closeModelDetail}
        modelData={pricingData.selectedModel}
        groupRatio={pricingData.groupRatio}
        groupNames={pricingData.groupNames}
        usableGroup={pricingData.usableGroup}
        currency={pricingData.currency}
        siteDisplayType={pricingData.siteDisplayType}
        tokenUnit={pricingData.tokenUnit}
        priceRate={pricingData.priceRate}
        usdExchangeRate={pricingData.usdExchangeRate}
        displayPrice={pricingData.displayPrice}
        showRatio={allProps.showRatio}
        vendorsMap={pricingData.vendorsMap}
        endpointMap={pricingData.endpointMap}
        autoGroups={pricingData.autoGroups}
        performance={
          pricingData.performanceMap[pricingData.selectedModel?.model_name]
        }
        t={pricingData.t}
      />

      <BillingGuide
        visible={billingGuideVisible}
        onClose={() => setBillingGuideVisible(false)}
        isMobile={isMobile}
        models={pricingData.models}
        groupRatio={pricingData.groupRatio}
        groupNames={pricingData.groupNames}
        selectedGroup={pricingData.selectedGroup}
        currency={pricingData.currency}
        siteDisplayType={pricingData.siteDisplayType}
        priceRate={pricingData.priceRate}
        usdExchangeRate={pricingData.usdExchangeRate}
        customExchangeRate={pricingData.customExchangeRate}
        customCurrencySymbol={pricingData.customCurrencySymbol}
        t={pricingData.t}
      />
    </div>
  );
};

export default PricingPage;
