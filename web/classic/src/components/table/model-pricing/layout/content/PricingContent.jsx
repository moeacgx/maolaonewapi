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
import PricingSidebar from '../PricingSidebar';
import PricingFilterModal from '../../modal/PricingFilterModal';
import PricingMarketplaceHeader from '../header/PricingMarketplaceHeader';
import PricingToolbar from '../PricingToolbar';
import PricingView from './PricingView';

const PricingContent = ({ isMobile, sidebarProps, ...props }) => {
  const [filterModalVisible, setFilterModalVisible] = React.useState(false);

  return (
    <div className='classic-pricing-page'>
      <div className='classic-pricing-shell'>
        <PricingMarketplaceHeader
          models={props.models}
          filteredModels={props.filteredModels}
          filterVendor={sidebarProps.filterVendor}
          searchValue={props.searchValue}
          handleChange={props.handleChange}
          handleCompositionStart={props.handleCompositionStart}
          handleCompositionEnd={props.handleCompositionEnd}
          t={props.t}
        />

        <div className='classic-pricing-workspace'>
          <PricingSidebar {...sidebarProps} />

          <main className='classic-pricing-results'>
            <PricingToolbar
              models={props.models}
              filteredModels={props.filteredModels}
              selectedRowKeys={props.selectedRowKeys}
              setSelectedRowKeys={props.setSelectedRowKeys}
              selectionMode={props.selectionMode}
              setSelectionMode={props.setSelectionMode}
              copyText={props.copyText}
              onOpenFilters={() => setFilterModalVisible(true)}
              isMobile={isMobile}
              searchValue={props.searchValue}
              filterGroup={sidebarProps.filterGroup}
              filterQuotaType={sidebarProps.filterQuotaType}
              filterEndpointType={sidebarProps.filterEndpointType}
              filterVendor={sidebarProps.filterVendor}
              filterTag={sidebarProps.filterTag}
              showWithRecharge={sidebarProps.showWithRecharge}
              setShowWithRecharge={sidebarProps.setShowWithRecharge}
              currency={sidebarProps.currency}
              setCurrency={sidebarProps.setCurrency}
              siteDisplayType={sidebarProps.siteDisplayType}
              showRatio={sidebarProps.showRatio}
              setShowRatio={sidebarProps.setShowRatio}
              viewMode={sidebarProps.viewMode}
              setViewMode={sidebarProps.setViewMode}
              tokenUnit={sidebarProps.tokenUnit}
              setTokenUnit={sidebarProps.setTokenUnit}
              sortBy={sidebarProps.sortBy}
              setSortBy={sidebarProps.setSortBy}
              onOpenBillingGuide={props.onOpenBillingGuide}
              billingWelcomeVisible={props.billingWelcomeVisible}
              onCloseBillingWelcome={props.onCloseBillingWelcome}
              onDismissBillingWelcome={props.onDismissBillingWelcome}
              t={props.t}
            />

            <div className='classic-pricing-view-container'>
              <PricingView {...props} viewMode={sidebarProps.viewMode} />
            </div>
          </main>
        </div>
      </div>

      <PricingFilterModal
        visible={filterModalVisible}
        onClose={() => setFilterModalVisible(false)}
        sidebarProps={sidebarProps}
        isMobile={isMobile}
        t={props.t}
      />
    </div>
  );
};

export default PricingContent;
