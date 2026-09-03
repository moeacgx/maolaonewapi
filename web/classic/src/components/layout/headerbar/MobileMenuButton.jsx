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
import { Button } from '@douyinfe/semi-ui';
import { IconClose, IconMenu } from '@douyinfe/semi-icons';
import {
  getConsoleSidebarToggleState,
  shouldRenderConsoleSidebarToggle,
} from './consoleHeaderBehavior';

const MobileMenuButton = ({
  isConsoleRoute,
  isMobile,
  drawerOpen,
  collapsed,
  onToggle,
  t,
}) => {
  if (
    !shouldRenderConsoleSidebarToggle({
      isConsoleRoute,
      isMobile,
    })
  ) {
    return null;
  }

  const sidebarState = getConsoleSidebarToggleState({
    isMobile,
    drawerOpen,
    collapsed,
  });

  return (
    <Button
      icon={
        sidebarState.isOpen ? (
          <IconClose className='text-lg' />
        ) : (
          <IconMenu className='text-lg' />
        )
      }
      aria-label={t(sidebarState.ariaLabel)}
      onClick={onToggle}
      theme='borderless'
      type='tertiary'
      className='classic-console-sidebar-toggle !p-2 !text-current focus:!bg-semi-color-fill-1 dark:focus:!bg-gray-700'
    />
  );
};

export default MobileMenuButton;
