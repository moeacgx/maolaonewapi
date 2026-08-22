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
import { Menu, X } from 'lucide-react';
import { getCustomNavIcon } from '../../../helpers/customNav';
import LanguageSelector from './LanguageSelector';
import NotificationButton from './NotificationButton';
import ThemeToggle from './ThemeToggle';
import UserArea from './UserArea';
import './PricingTemplateHeader.css';

const PricingTemplateHeader = ({
  logo,
  logoLoaded,
  systemName,
  mainNavLinks,
  userState,
  pricingRequireAuth,
  unreadCount,
  onNoticeOpen,
  theme,
  onThemeToggle,
  currentLang,
  onLanguageChange,
  isLoading,
  isSelfUseMode,
  logout,
  t,
}) => {
  const [mobileOpen, setMobileOpen] = React.useState(false);
  const [scrolled, setScrolled] = React.useState(false);

  React.useEffect(() => {
    const scrollContainer = document.querySelector('section.semi-layout');
    const handleScroll = () => {
      setScrolled(
        (scrollContainer?.scrollTop || 0) > 20 || window.scrollY > 20,
      );
    };
    const closeOnDesktop = () => {
      if (window.innerWidth >= 640) setMobileOpen(false);
    };

    handleScroll();
    window.addEventListener('scroll', handleScroll, { passive: true });
    scrollContainer?.addEventListener('scroll', handleScroll, {
      passive: true,
    });
    window.addEventListener('resize', closeOnDesktop);
    return () => {
      window.removeEventListener('scroll', handleScroll);
      scrollContainer?.removeEventListener('scroll', handleScroll);
      window.removeEventListener('resize', closeOnDesktop);
    };
  }, []);

  React.useEffect(() => {
    const previousOverflow = document.body.style.overflow;
    document.body.style.overflow = mobileOpen ? 'hidden' : previousOverflow;
    return () => {
      document.body.style.overflow = previousOverflow;
    };
  }, [mobileOpen]);

  const getLinkHref = React.useCallback(
    (link) => {
      const requiresLogin =
        !userState.user &&
        (link.itemKey === 'console' ||
          (link.itemKey === 'pricing' && pricingRequireAuth) ||
          link.requiresAuth);

      if (requiresLogin) return '/login';
      return link.isExternal ? link.externalLink : link.to;
    },
    [pricingRequireAuth, userState.user],
  );

  const renderLink = (link, mobile = false) => {
    const Icon = link.iconName ? getCustomNavIcon(link.iconName) : null;
    const href = getLinkHref(link);
    const isActive = !link.isExternal && window.location.pathname === link.to;
    const className = mobile
      ? `classic-pricing-template-mobile-link${isActive ? ' is-active' : ''}`
      : `classic-pricing-template-nav-link${isActive ? ' is-active' : ''}`;

    return (
      <a
        key={link.itemKey}
        className={className}
        href={href}
        target={link.isExternal ? '_blank' : undefined}
        rel={link.isExternal ? 'noopener noreferrer' : undefined}
        onClick={() => {
          if (mobile) setMobileOpen(false);
        }}
      >
        {Icon && <Icon size={mobile ? 20 : 16} aria-hidden='true' />}
        <span>{link.text}</span>
      </a>
    );
  };

  const isAuthenticated = Boolean(userState.user);

  return (
    <>
      <header className='classic-pricing-template-header'>
        <div
          className={`classic-pricing-template-header-frame${
            scrolled ? ' is-scrolled' : ''
          }`}
        >
          <nav className='classic-pricing-template-nav'>
            <a className='classic-pricing-template-brand' href='/'>
              <span className='classic-pricing-template-logo-wrap'>
                {logoLoaded && (
                  <img
                    src={logo}
                    alt={systemName}
                    className='classic-pricing-template-logo'
                  />
                )}
              </span>
              <span className='classic-pricing-template-site-name'>
                {systemName}
              </span>
            </a>

            <div className='classic-pricing-template-desktop-nav'>
              {mainNavLinks.map((link) => renderLink(link))}
              <span className='classic-pricing-template-nav-divider' />
              <LanguageSelector
                currentLang={currentLang}
                onLanguageChange={onLanguageChange}
                t={t}
                bare
              />
              <ThemeToggle
                theme={theme}
                onThemeToggle={onThemeToggle}
                t={t}
                preferActualIcon
                bare
              />
              <NotificationButton
                unreadCount={unreadCount}
                onNoticeOpen={onNoticeOpen}
                t={t}
                bare
              />
              <span className='classic-pricing-template-nav-divider is-auth' />
              {isAuthenticated ? (
                <UserArea
                  userState={userState}
                  isLoading={isLoading}
                  isMobile={false}
                  isSelfUseMode={isSelfUseMode}
                  logout={logout}
                  t={t}
                />
              ) : (
                <a className='classic-pricing-template-login' href='/login'>
                  {t('登录')}
                </a>
              )}
            </div>

            <div className='classic-pricing-template-mobile-actions'>
              <ThemeToggle
                theme={theme}
                onThemeToggle={onThemeToggle}
                t={t}
                preferActualIcon
                bare
              />
              <button
                type='button'
                className='classic-pricing-template-menu-button'
                onClick={() => setMobileOpen((open) => !open)}
                aria-label={t('切换导航菜单')}
                aria-expanded={mobileOpen}
                aria-controls='classic-pricing-template-mobile-menu'
              >
                {mobileOpen ? <X size={18} /> : <Menu size={18} />}
              </button>
            </div>
          </nav>
        </div>
      </header>

      <div
        id='classic-pricing-template-mobile-menu'
        className={`classic-pricing-template-mobile-menu${
          mobileOpen ? ' is-open' : ''
        }`}
        aria-hidden={!mobileOpen}
      >
        <nav className='classic-pricing-template-mobile-links'>
          {mainNavLinks.map((link) => renderLink(link, true))}
        </nav>
        <a
          className='classic-pricing-template-mobile-login'
          href={isAuthenticated ? '/console' : '/login'}
          tabIndex={mobileOpen ? undefined : -1}
          onClick={() => setMobileOpen(false)}
        >
          {isAuthenticated ? t('控制台') : t('登录')}
        </a>
      </div>
    </>
  );
};

export default PricingTemplateHeader;
