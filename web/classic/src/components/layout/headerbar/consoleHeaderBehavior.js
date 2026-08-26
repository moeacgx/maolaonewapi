export const isConsoleHomePath = (pathname) =>
  pathname === '/console' || pathname === '/console/';

export const shouldRenderConsoleSidebarToggle = ({
  isConsoleRoute,
  isMobile,
  showOnDesktop,
}) => Boolean(isConsoleRoute && (isMobile || showOnDesktop));

export const getConsoleSidebarToggleState = ({
  isMobile,
  drawerOpen,
  collapsed,
}) => {
  const isOpen = isMobile ? drawerOpen : !collapsed;

  return {
    isOpen,
    ariaLabel: isOpen ? '关闭侧边栏' : '打开侧边栏',
  };
};
