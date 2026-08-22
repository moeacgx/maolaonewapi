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
import { Card, Skeleton } from '@douyinfe/semi-ui';

const PricingCardSkeleton = ({
  skeletonCount = 12,
  rowSelection = false,
  showRatio = false,
  isMobile = false,
}) => {
  const placeholder = (
    <div className='classic-pricing-card-list'>
      <div className='classic-pricing-card-grid'>
        {Array.from({ length: skeletonCount }).map((_, index) => (
          <Card
            key={index}
            className='classic-pricing-model-card'
            bodyStyle={{ padding: isMobile ? 14 : 16 }}
          >
            <div className='classic-pricing-model-card-body'>
              <div className='classic-pricing-model-card-header'>
                <div className='classic-pricing-model-card-title-wrap'>
                  <div className='classic-pricing-skeleton-icon'>
                    <Skeleton.Avatar size='small' />
                  </div>
                  <div className='classic-pricing-skeleton-title-content'>
                    <Skeleton.Title
                      style={{
                        width: `${120 + (index % 3) * 30}px`,
                        height: 18,
                        marginBottom: 7,
                      }}
                    />
                    <Skeleton.Title
                      style={{
                        width: `${160 + (index % 4) * 18}px`,
                        height: 14,
                        marginBottom: 0,
                      }}
                    />
                  </div>
                </div>
                <div className='classic-pricing-skeleton-actions'>
                  <Skeleton.Button
                    size='small'
                    style={{ width: 24, height: 24, borderRadius: 6 }}
                  />
                  {rowSelection && (
                    <Skeleton.Button
                      size='small'
                      style={{ width: 16, height: 16, borderRadius: 3 }}
                    />
                  )}
                </div>
              </div>

              <div className='classic-pricing-skeleton-description'>
                <Skeleton.Title
                  style={{ width: '90%', height: 13, marginBottom: 6 }}
                />
                <Skeleton.Title
                  style={{ width: '58%', height: 13, marginBottom: 0 }}
                />
              </div>

              <div className='classic-pricing-model-card-footer'>
                <div className='classic-pricing-skeleton-footer'>
                  <Skeleton.Button
                    size='small'
                    style={{ width: 64, height: 18, borderRadius: 10 }}
                  />
                  <Skeleton.Title
                    style={{ width: 114, height: 12, marginBottom: 0 }}
                  />
                </div>

                {showRatio && (
                  <div className='classic-pricing-model-card-ratios'>
                    <Skeleton.Title
                      style={{ width: 68, height: 12, marginBottom: 8 }}
                    />
                    <div className='classic-pricing-model-card-ratio-grid'>
                      {Array.from({ length: 3 }).map((_, ratioIndex) => (
                        <Skeleton.Title
                          key={ratioIndex}
                          style={{
                            width: '100%',
                            height: 28,
                            marginBottom: 0,
                          }}
                        />
                      ))}
                    </div>
                  </div>
                )}
              </div>
            </div>
          </Card>
        ))}
      </div>

      <div className='flex justify-center mt-6 py-4 border-t pricing-pagination-divider'>
        <Skeleton.Button style={{ width: 300, height: 32 }} />
      </div>
    </div>
  );

  return <Skeleton loading={true} active placeholder={placeholder}></Skeleton>;
};

export default PricingCardSkeleton;
