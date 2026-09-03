export const isConsolePath = (pathname) =>
  pathname === '/console' || pathname.startsWith('/console/');

export const isConsoleHomePath = (pathname) =>
  pathname === '/console' || pathname === '/console/';

export const shouldRenderConsoleSidebarToggle = ({
  isConsoleRoute,
  isMobile,
}) => Boolean(isConsoleRoute && isMobile);

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
