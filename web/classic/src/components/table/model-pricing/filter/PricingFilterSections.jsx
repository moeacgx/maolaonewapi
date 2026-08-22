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
import PricingDisplaySettings from './PricingDisplaySettings';
import PricingEndpointTypes from './PricingEndpointTypes';
import PricingGroups from './PricingGroups';
import PricingQuotaTypes from './PricingQuotaTypes';
import PricingTags from './PricingTags';
import PricingVendors from './PricingVendors';
import { usePricingFilterCounts } from '../../../../hooks/model-pricing/usePricingFilterCounts';

const PricingFilterSections = ({
  includeDisplaySettings = false,
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
  filterGroup,
  setFilterGroup,
  handleGroupClick,
  filterQuotaType,
  setFilterQuotaType,
  filterEndpointType,
  setFilterEndpointType,
  filterVendor,
  setFilterVendor,
  filterTag,
  setFilterTag,
  models = [],
  vendors = [],
  searchValue,
  usableGroup,
  groupRatio,
  groupNames,
  loading,
  t,
}) => {
  const {
    quotaTypeModels,
    endpointTypeModels,
    vendorModels,
    tagModels,
    groupCountModels,
  } = usePricingFilterCounts({
    models,
    filterGroup,
    filterQuotaType,
    filterEndpointType,
    filterVendor,
    filterTag,
    searchValue,
  });

  return (
    <div className='classic-pricing-filter-sections'>
      {includeDisplaySettings && (
        <PricingDisplaySettings
          showWithRecharge={showWithRecharge}
          setShowWithRecharge={setShowWithRecharge}
          currency={currency}
          setCurrency={setCurrency}
          siteDisplayType={siteDisplayType}
          showRatio={showRatio}
          setShowRatio={setShowRatio}
          viewMode={viewMode}
          setViewMode={setViewMode}
          tokenUnit={tokenUnit}
          setTokenUnit={setTokenUnit}
          loading={loading}
          t={t}
        />
      )}

      <PricingGroups
        filterGroup={filterGroup}
        setFilterGroup={handleGroupClick || setFilterGroup}
        usableGroup={usableGroup}
        groupRatio={groupRatio}
        groupNames={groupNames}
        models={groupCountModels}
        loading={loading}
        t={t}
      />

      <PricingVendors
        filterVendor={filterVendor}
        setFilterVendor={setFilterVendor}
        models={vendorModels}
        allModels={models}
        vendors={vendors}
        loading={loading}
        t={t}
      />

      <PricingTags
        filterTag={filterTag}
        setFilterTag={setFilterTag}
        models={tagModels}
        allModels={models}
        loading={loading}
        t={t}
      />

      <PricingQuotaTypes
        filterQuotaType={filterQuotaType}
        setFilterQuotaType={setFilterQuotaType}
        models={quotaTypeModels}
        loading={loading}
        t={t}
      />

      <PricingEndpointTypes
        filterEndpointType={filterEndpointType}
        setFilterEndpointType={setFilterEndpointType}
        models={endpointTypeModels}
        allModels={models}
        loading={loading}
        t={t}
      />
    </div>
  );
};

export default PricingFilterSections;
