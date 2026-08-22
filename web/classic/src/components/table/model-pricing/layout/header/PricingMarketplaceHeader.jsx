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
import { Input } from '@douyinfe/semi-ui';
import { IconSearch } from '@douyinfe/semi-icons';

const PricingMarketplaceHeader = ({
  models = [],
  searchValue,
  handleChange,
  handleCompositionStart,
  handleCompositionEnd,
  t,
}) => {
  React.useEffect(() => {
    const handleKeyDown = (event) => {
      const searchInput = document.querySelector(
        '.classic-pricing-marketplace-search input',
      );

      if ((event.metaKey || event.ctrlKey) && event.key.toLowerCase() === 'k') {
        event.preventDefault();
        searchInput?.focus();
      }

      if (event.key === 'Escape' && document.activeElement === searchInput) {
        searchInput.blur();
      }
    };

    document.addEventListener('keydown', handleKeyDown);
    return () => document.removeEventListener('keydown', handleKeyDown);
  }, []);

  return (
    <header className='classic-pricing-marketplace-header'>
      <h1 className='classic-pricing-marketplace-title'>{t('模型广场')}</h1>
      <p className='classic-pricing-marketplace-count'>
        {t('本站当前已启用模型，总计 {{count}} 个', {
          count: models.length,
        })}
      </p>
      <p className='classic-pricing-marketplace-description'>
        {t('探索精选 AI 模型，清晰比较价格与能力，为不同场景选择合适的模型。')}
      </p>

      <Input
        className='classic-pricing-marketplace-search'
        prefix={<IconSearch />}
        suffix={<kbd className='classic-pricing-search-shortcut'>⌘K</kbd>}
        placeholder={t('搜索模型名称、供应商、端点或标签...')}
        value={searchValue}
        onCompositionStart={handleCompositionStart}
        onCompositionEnd={handleCompositionEnd}
        onChange={handleChange}
        showClear
        aria-label={t('搜索模型')}
      />
    </header>
  );
};

export default PricingMarketplaceHeader;
