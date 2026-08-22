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
import { Skeleton } from '@douyinfe/semi-ui';
import { IconChevronDown, IconChevronUp } from '@douyinfe/semi-icons';

const DEFAULT_VISIBLE_ITEM_COUNT = Number.POSITIVE_INFINITY;

const PricingFilterSection = ({
  title,
  items = [],
  activeValue,
  onChange,
  loading = false,
  collapsible = true,
  visibleItemCount = DEFAULT_VISIBLE_ITEM_COUNT,
  t,
}) => {
  const [sectionOpen, setSectionOpen] = React.useState(true);
  const [showAllItems, setShowAllItems] = React.useState(false);
  const isMultiple = Array.isArray(activeValue);
  const canCollapse = collapsible && items.length > visibleItemCount;
  const visibleItems =
    showAllItems || !canCollapse ? items : items.slice(0, visibleItemCount);

  return (
    <section className='classic-pricing-filter-section' aria-label={title}>
      <h3 className='classic-pricing-filter-section-title'>
        {loading ? (
          <Skeleton.Title style={{ width: 88, height: 14 }} />
        ) : (
          <button
            type='button'
            className='classic-pricing-filter-section-trigger'
            aria-expanded={sectionOpen}
            onClick={() => setSectionOpen((current) => !current)}
          >
            <span className='classic-pricing-filter-section-title-content'>
              <span className='classic-pricing-filter-section-label'>
                {title}
              </span>
            </span>
            {sectionOpen ? (
              <IconChevronUp size='small' aria-hidden='true' />
            ) : (
              <IconChevronDown size='small' aria-hidden='true' />
            )}
          </button>
        )}
      </h3>

      {sectionOpen && (
        <>
          <div
            className='classic-pricing-filter-options'
            role='group'
            aria-label={title}
          >
            {loading
              ? Array.from({ length: 6 }).map((_, index) => (
                  <Skeleton.Title
                    key={index}
                    style={{
                      width: `${72 + (index % 3) * 18}px`,
                      height: 28,
                      marginBottom: 0,
                    }}
                  />
                ))
              : visibleItems.map((item) => {
                  const active = isMultiple
                    ? activeValue.includes(item.value)
                    : activeValue === item.value;
                  const hasMeta =
                    item.tagCount !== undefined && item.tagCount !== '';

                  return (
                    <button
                      key={item.value}
                      type='button'
                      title={item.label}
                      className={`classic-pricing-filter-chip${
                        active ? ' is-active' : ''
                      }`}
                      aria-pressed={active}
                      onClick={() => onChange(item.value)}
                    >
                      {item.icon && (
                        <span className='classic-pricing-filter-chip-icon'>
                          {item.icon}
                        </span>
                      )}
                      <span className='classic-pricing-filter-chip-label'>
                        {item.label}
                      </span>
                      {hasMeta && (
                        <span className='classic-pricing-filter-chip-meta'>
                          {item.tagCount}
                        </span>
                      )}
                    </button>
                  );
                })}
          </div>

          {canCollapse && !loading && (
            <button
              type='button'
              className='classic-pricing-filter-expand'
              aria-expanded={showAllItems}
              onClick={() => setShowAllItems((current) => !current)}
            >
              {showAllItems ? (
                <IconChevronUp size='small' />
              ) : (
                <IconChevronDown size='small' />
              )}
              <span>{showAllItems ? t('收起') : t('展开更多')}</span>
            </button>
          )}
        </>
      )}
    </section>
  );
};

export default PricingFilterSection;
