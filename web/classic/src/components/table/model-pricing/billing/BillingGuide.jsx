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

import React, {
  useCallback,
  useEffect,
  useMemo,
  useRef,
  useState,
} from 'react';
import {
  Button,
  Divider,
  InputNumber,
  Modal,
  Select,
  Tag,
} from '@douyinfe/semi-ui';
import {
  IconArrowLeft,
  IconArrowRight,
  IconHelpCircle,
  IconRefresh,
} from '@douyinfe/semi-icons';
import { calculateModelPrice, getGroupDisplayName } from '../../../../helpers';
import { BILLING_GUIDE_MASK_STYLE } from './BillingGuideWelcome';
import {
  DEFAULT_TOKEN_COUNTS,
  calculateTokenCost,
  formatBillingMoney,
  formatBillingNumber,
  getBillingDiscountColor,
  getBillingDiscountText,
  getBillingDynamicUnitPrices,
  getBillingFactors,
  getBillingGuideGroups,
  getBillingGuideModels,
  getBillingCurrency,
  getBillingUnitPricesFromPriceData,
  pickBillingGuideGroup,
  pickBillingGuideModel,
} from './utils';

const CARD_STYLE = {
  border: '1px solid var(--semi-color-border)',
  borderRadius: 12,
  backgroundColor: 'var(--semi-color-bg-0)',
};

const ACTIVE_CARD_STYLE = {
  ...CARD_STYLE,
  borderColor: 'var(--semi-color-primary)',
  backgroundColor: 'var(--semi-color-primary-light-default)',
  boxShadow: '0 0 0 1px var(--semi-color-primary-light-active)',
};

const stripTrailingZeros = (value) =>
  String(value)
    .replace(/(\.\d*?[1-9])0+$/u, '$1')
    .replace(/\.0+$/u, '');

const formatFixedNumber = (value, digits = 4) =>
  stripTrailingZeros(Number(value || 0).toFixed(digits));

const formatUnitPrice = (symbol, value) =>
  `${symbol}${formatFixedNumber(value, 6)}/M`;

const formatExactMoney = (symbol, value) =>
  `${symbol}${Number(value || 0).toFixed(9)}`;

const PriceLine = ({ label, price, officialPrice, symbol, isMobile }) => {
  if (!price) return null;

  return (
    <div
      className={`grid items-center gap-2 border-t py-2.5 text-sm first:border-t-0 ${
        isMobile
          ? 'grid-cols-[minmax(0,1fr)_auto_minmax(0,1fr)]'
          : 'grid-cols-[minmax(72px,1fr)_auto_auto_auto]'
      }`}
    >
      <span
        className={`min-w-0 ${isMobile ? 'col-span-3' : ''}`}
        style={{ color: 'var(--semi-color-text-2)' }}
      >
        {label}
      </span>
      <span
        className={`font-mono text-xs line-through ${isMobile ? 'text-left' : ''}`}
        style={{ color: 'var(--semi-color-text-3)' }}
      >
        {formatUnitPrice(symbol, officialPrice)}
      </span>
      <IconArrowRight
        aria-hidden='true'
        size='small'
        style={{ color: 'var(--semi-color-text-3)' }}
      />
      <strong
        className='font-mono text-right'
        style={{ color: 'var(--semi-color-primary)' }}
      >
        {formatUnitPrice(symbol, price.unitPrice)}
      </strong>
    </div>
  );
};

const ModelGroupSelector = ({
  isMobile,
  modelOptions,
  groupOptions,
  selectedModelName,
  selectedGroupName,
  onModelChange,
  onGroupChange,
  t,
}) => (
  <div className={`flex gap-2 ${isMobile ? 'flex-col' : 'items-center'}`}>
    <div className='flex min-w-0 flex-1 items-center gap-2'>
      <span
        id='billing-guide-model-label'
        className='shrink-0 text-xs'
        style={{ color: 'var(--semi-color-text-2)' }}
      >
        {t('模型')}
      </span>
      <Select
        className='min-w-0 flex-1'
        aria-labelledby='billing-guide-model-label'
        value={selectedModelName}
        optionList={modelOptions}
        onChange={onModelChange}
        filter
        placeholder={t('选择模型')}
      />
    </div>
    <div className='flex min-w-0 flex-1 items-center gap-2'>
      <span
        id='billing-guide-group-label'
        className='shrink-0 text-xs'
        style={{ color: 'var(--semi-color-text-2)' }}
      >
        {t('分组')}
      </span>
      <Select
        className='min-w-0 flex-1'
        aria-labelledby='billing-guide-group-label'
        value={selectedGroupName}
        optionList={groupOptions}
        onChange={onGroupChange}
        placeholder={t('选择分组')}
      />
    </div>
  </div>
);

const BillingGuide = ({
  visible,
  onClose,
  isMobile,
  models = [],
  groupRatio = {},
  groupNames = {},
  selectedGroup = 'all',
  currency = 'USD',
  siteDisplayType = 'USD',
  priceRate = 1,
  usdExchangeRate = 1,
  customExchangeRate = 1,
  customCurrencySymbol = '¤',
  t,
}) => {
  const [step, setStep] = useState(0);
  const [selectedModelName, setSelectedModelName] = useState('');
  const [selectedGroupName, setSelectedGroupName] = useState('');
  const [tokenCounts, setTokenCounts] = useState(DEFAULT_TOKEN_COUNTS);
  const [animatedTotal, setAnimatedTotal] = useState(0);
  const [animationKey, setAnimationKey] = useState(0);
  const initializedForOpenRef = useRef(false);
  const scrollContainerRef = useRef(null);

  const guideModels = useMemo(() => getBillingGuideModels(models), [models]);
  const selectedModel = useMemo(
    () =>
      guideModels.find((model) => model.model_name === selectedModelName) ||
      null,
    [guideModels, selectedModelName],
  );
  const groups = useMemo(
    () => getBillingGuideGroups(selectedModel, groupRatio),
    [selectedModel, groupRatio],
  );
  const selectedGroupInfo = useMemo(
    () =>
      groups.find((group) => group.value === selectedGroupName) || groups[0],
    [groups, selectedGroupName],
  );

  const effectiveCurrency = siteDisplayType === 'TOKENS' ? 'USD' : currency;
  const factors = useMemo(
    () =>
      getBillingFactors({
        groupRatio: selectedGroupInfo?.ratio ?? 1,
        priceRate,
        usdExchangeRate,
      }),
    [selectedGroupInfo, priceRate, usdExchangeRate],
  );
  const currencyMeta = useMemo(
    () =>
      getBillingCurrency({
        currency: effectiveCurrency,
        usdExchangeRate,
        customExchangeRate,
        customCurrencySymbol,
      }),
    [
      effectiveCurrency,
      usdExchangeRate,
      customExchangeRate,
      customCurrencySymbol,
    ],
  );
  const rechargeDisplayPrice = useCallback(
    (usdPrice) => {
      const normalizedPrice = Number(usdPrice);
      const displayValue = Number.isFinite(normalizedPrice)
        ? normalizedPrice * factors.forexFactor * currencyMeta.multiplier
        : 0;
      return `${currencyMeta.symbol}${displayValue.toFixed(6)}`;
    },
    [currencyMeta, factors.forexFactor],
  );
  const priceData = useMemo(
    () =>
      selectedModel
        ? calculateModelPrice({
            record: selectedModel,
            selectedGroup: selectedGroupName,
            groupRatio,
            tokenUnit: 'M',
            displayPrice: rechargeDisplayPrice,
            currency: effectiveCurrency,
            quotaDisplayType:
              siteDisplayType === 'TOKENS' ? 'USD' : siteDisplayType,
            precision: 6,
          })
        : null,
    [
      selectedModel,
      selectedGroupName,
      groupRatio,
      rechargeDisplayPrice,
      effectiveCurrency,
      siteDisplayType,
    ],
  );
  const prices = useMemo(() => {
    if (priceData?.isDynamicPricing) {
      return getBillingDynamicUnitPrices({
        priceData,
        tokenCounts,
        displayPrice: rechargeDisplayPrice,
        currency: effectiveCurrency,
        usdExchangeRate,
        customExchangeRate,
        customCurrencySymbol,
      });
    }
    return getBillingUnitPricesFromPriceData({
      priceData,
      currency: effectiveCurrency,
      usdExchangeRate,
      customExchangeRate,
      customCurrencySymbol,
    });
  }, [
    priceData,
    tokenCounts,
    rechargeDisplayPrice,
    effectiveCurrency,
    usdExchangeRate,
    customExchangeRate,
    customCurrencySymbol,
  ]);

  const modelOptions = useMemo(
    () =>
      guideModels.map((model) => ({
        value: model.model_name,
        label: model.model_name,
      })),
    [guideModels],
  );
  const groupOptions = useMemo(
    () =>
      groups.map((group) => ({
        value: group.value,
        label: `${getGroupDisplayName(group.value, groupNames)} · ×${formatFixedNumber(group.ratio, 3)}`,
      })),
    [groupNames, groups],
  );

  useEffect(() => {
    if (!visible) {
      initializedForOpenRef.current = false;
      return;
    }
    if (initializedForOpenRef.current || guideModels.length === 0) return;

    const model = pickBillingGuideModel(guideModels);
    const group = pickBillingGuideGroup(model, groupRatio, selectedGroup);
    setSelectedModelName(model?.model_name || '');
    setSelectedGroupName(group?.value || '');
    setTokenCounts(DEFAULT_TOKEN_COUNTS);
    setStep(0);
    setAnimationKey((value) => value + 1);
    initializedForOpenRef.current = true;
  }, [visible, guideModels, groupRatio, selectedGroup]);

  useEffect(() => {
    if (!visible) return undefined;
    const frameId = requestAnimationFrame(() => {
      scrollContainerRef.current?.scrollTo({ top: 0 });
    });
    return () => cancelAnimationFrame(frameId);
  }, [visible, step]);

  const handleModelChange = useCallback(
    (modelName) => {
      const model = guideModels.find((item) => item.model_name === modelName);
      const group = pickBillingGuideGroup(model, groupRatio, selectedGroupName);
      setSelectedModelName(modelName);
      setSelectedGroupName(group?.value || '');
    },
    [guideModels, groupRatio, selectedGroupName],
  );

  const costRows = useMemo(() => {
    if (!prices) return [];
    return [
      {
        key: 'input',
        label: t('输入'),
        description: t('未命中缓存的 prompt token'),
        price: prices.input,
      },
      {
        key: 'output',
        label: t('输出'),
        description: t('模型生成的 completion token'),
        price: prices.output,
      },
      {
        key: 'cacheRead',
        label: t('缓存读取'),
        description: t('命中缓存的 prompt token'),
        price: prices.cacheRead,
        badge: t('读'),
      },
      {
        key: 'cacheWrite',
        label: t('缓存创建'),
        description: t('本次写入缓存的 token'),
        price: prices.cacheWrite,
        badge: t('写'),
      },
    ]
      .filter((row) => row.price)
      .map((row) => ({
        ...row,
        cost: calculateTokenCost(tokenCounts[row.key], row.price.unitPrice),
      }));
  }, [prices, tokenCounts, t]);

  const totalCost = useMemo(
    () => costRows.reduce((total, row) => total + row.cost, 0),
    [costRows],
  );

  const formulaRows = useMemo(
    () =>
      costRows.map((row) => ({
        ...row,
        exactCost: formatExactMoney(prices?.symbol || '$', row.cost),
        displayedCost: formatBillingMoney(prices?.symbol || '$', row.cost, 6),
        displayedUnitPrice: formatBillingMoney(
          prices?.symbol || '$',
          row.price.unitPrice,
          6,
        ),
      })),
    [costRows, prices?.symbol],
  );

  const primaryPrice = formulaRows[0]?.price || null;
  const displayMultiplier = prices?.multiplier || 1;
  const primaryOfficialUsdPrice = primaryPrice
    ? primaryPrice.officialPrice / displayMultiplier
    : 0;
  const discountText = getBillingDiscountText(factors.compositeFactor, t, 2);
  const fullUnitFormula = t(
    '官方美元单价 ×（充值汇率 ÷ 美元汇率）× 分组倍率 × 展示货币汇率',
  );
  const totalEquation = formulaRows.map((row) => row.exactCost).join(' + ');

  useEffect(() => {
    if (!visible || step !== 1) return undefined;
    if (
      typeof window !== 'undefined' &&
      window.matchMedia?.('(prefers-reduced-motion: reduce)').matches
    ) {
      setAnimatedTotal(totalCost);
      return undefined;
    }

    let frameId;
    const startedAt = performance.now();
    const duration = 700;
    const animate = (now) => {
      const progress = Math.min(1, (now - startedAt) / duration);
      const easedProgress = 1 - Math.pow(1 - progress, 3);
      setAnimatedTotal(totalCost * easedProgress);
      if (progress < 1) frameId = requestAnimationFrame(animate);
    };
    setAnimatedTotal(0);
    frameId = requestAnimationFrame(animate);

    return () => cancelAnimationFrame(frameId);
  }, [visible, step, totalCost, animationKey]);

  const handleTokenChange = useCallback((key, value) => {
    const normalizedValue = Math.max(0, Math.floor(Number(value) || 0));
    setTokenCounts((current) => ({ ...current, [key]: normalizedValue }));
  }, []);

  const handleReplay = useCallback(() => {
    setTokenCounts(DEFAULT_TOKEN_COUNTS);
    setAnimationKey((value) => value + 1);
  }, []);

  const renderSelector = () => (
    <ModelGroupSelector
      isMobile={isMobile}
      modelOptions={modelOptions}
      groupOptions={groupOptions}
      selectedModelName={selectedModelName}
      selectedGroupName={selectedGroupName}
      onModelChange={handleModelChange}
      onGroupChange={setSelectedGroupName}
      t={t}
    />
  );

  const renderStepOne = () => (
    <div>
      <div className='mb-4'>{renderSelector()}</div>

      {selectedModel && prices ? (
        <>
          <section className='p-4' style={ACTIVE_CARD_STYLE}>
            <div className='flex items-center justify-between gap-3'>
              <strong
                className='min-w-0 truncate text-base'
                style={{ color: 'var(--semi-color-text-0)' }}
              >
                {selectedModel.model_name}
              </strong>
              <Tag
                color={priceData.isDynamicPricing ? 'orange' : 'purple'}
                shape='circle'
              >
                {priceData.isDynamicPricing ? t('动态计费') : t('按量计费')}
              </Tag>
            </div>

            <div className='mt-4 grid grid-cols-[minmax(0,1fr)_auto_minmax(0,1fr)_auto_minmax(0,1fr)] items-center gap-2'>
              <div className='min-w-0 text-center'>
                <div
                  className='text-xs'
                  style={{ color: 'var(--semi-color-text-2)' }}
                >
                  {t('汇率优惠')}
                </div>
                <strong
                  className={`mt-1 block font-mono ${isMobile ? 'text-sm' : 'text-lg'}`}
                >
                  {formatBillingNumber(factors.forexFactor, 6)}
                </strong>
                {!isMobile && (
                  <div
                    className='mt-1 font-mono text-xs'
                    style={{ color: 'var(--semi-color-text-3)' }}
                  >
                    {formatFixedNumber(priceRate, 3)} ÷{' '}
                    {formatFixedNumber(usdExchangeRate, 3)}
                  </div>
                )}
              </div>

              <span
                aria-hidden='true'
                className='font-mono text-lg'
                style={{ color: 'var(--semi-color-text-3)' }}
              >
                ×
              </span>

              <div className='min-w-0 text-center'>
                <div
                  className='text-xs'
                  style={{ color: 'var(--semi-color-text-2)' }}
                >
                  {t('分组倍率')}
                </div>
                <strong
                  className={`mt-1 block font-mono ${isMobile ? 'text-sm' : 'text-lg'}`}
                >
                  {formatBillingNumber(factors.groupFactor, 6)}
                </strong>
              </div>

              <span
                aria-hidden='true'
                className='font-mono text-lg'
                style={{ color: 'var(--semi-color-text-3)' }}
              >
                =
              </span>

              <div className='min-w-0 text-center'>
                <div
                  className='text-xs font-semibold'
                  style={{ color: 'var(--semi-color-primary)' }}
                >
                  {t('综合折扣')}
                </div>
                <strong
                  className={`mt-1 block font-mono font-bold ${isMobile ? 'text-xl' : 'text-2xl'}`}
                  style={{ color: 'var(--semi-color-primary)' }}
                >
                  {discountText}
                </strong>
                <div
                  className='mt-1 truncate font-mono text-xs'
                  style={{ color: 'var(--semi-color-text-3)' }}
                >
                  {formatBillingNumber(factors.compositeFactor, 6)}
                </div>
              </div>
            </div>

            {isMobile && (
              <div
                className='mt-3 border-t pt-3 text-center font-mono text-[11px]'
                style={{ color: 'var(--semi-color-text-2)' }}
              >
                {formatFixedNumber(priceRate, 3)} ÷{' '}
                {formatFixedNumber(usdExchangeRate, 3)} ×{' '}
                {formatBillingNumber(factors.groupFactor, 6)} ={' '}
                {formatBillingNumber(factors.compositeFactor, 6)}
              </div>
            )}
          </section>

          <section className='mt-3 p-4' style={CARD_STYLE}>
            <div className='mb-1 flex items-center justify-between gap-3'>
              <strong>{t('当前充值单价')}</strong>
              {prices.dynamicTierLabel && (
                <span
                  className='text-xs'
                  style={{ color: 'var(--semi-color-text-2)' }}
                >
                  {t('命中档位')}：{prices.dynamicTierLabel}
                </span>
              )}
            </div>
            <PriceLine
              label={t('输入')}
              price={prices.input}
              officialPrice={prices.input?.officialPrice}
              symbol={prices.symbol}
              isMobile={isMobile}
            />
            <PriceLine
              label={t('输出')}
              price={prices.output}
              officialPrice={prices.output?.officialPrice}
              symbol={prices.symbol}
              isMobile={isMobile}
            />
            <PriceLine
              label={t('缓存读取')}
              price={prices.cacheRead}
              officialPrice={prices.cacheRead?.officialPrice}
              symbol={prices.symbol}
              isMobile={isMobile}
            />
            <PriceLine
              label={t('缓存创建')}
              price={prices.cacheWrite}
              officialPrice={prices.cacheWrite?.officialPrice}
              symbol={prices.symbol}
              isMobile={isMobile}
            />
          </section>

          <details className='mt-3 overflow-hidden' style={CARD_STYLE}>
            <summary className='cursor-pointer px-4 py-3 text-sm font-semibold'>
              {t('查看详情')}
            </summary>
            <div className='border-t px-4 pb-3'>
              <div className='flex items-start justify-between gap-3 py-2.5 text-sm'>
                <span>{t('充值汇率系数')}</span>
                <strong className='min-w-0 break-words font-mono text-right'>
                  {formatFixedNumber(priceRate, 3)} ÷{' '}
                  {formatFixedNumber(usdExchangeRate, 3)} ={' '}
                  {formatBillingNumber(factors.forexFactor, 6)}
                </strong>
              </div>
              <div className='flex items-start justify-between gap-3 border-t border-dashed py-2.5 text-sm'>
                <span>{t('分组倍率')}</span>
                <strong className='font-mono text-right'>
                  × {formatBillingNumber(factors.groupFactor, 6)}
                </strong>
              </div>
              <div className='flex items-start justify-between gap-3 border-t border-dashed py-2.5 text-sm'>
                <span>{t('展示货币汇率')}</span>
                <strong className='font-mono text-right'>
                  × {formatBillingNumber(currencyMeta.multiplier, 6)}
                </strong>
              </div>
              <div className='border-t border-dashed py-2.5 text-sm'>
                <div style={{ color: 'var(--semi-color-text-2)' }}>
                  {t('充值价格公式')}
                </div>
                <div className='mt-1 font-mono text-xs leading-5'>
                  {fullUnitFormula}
                </div>
                <div className='mt-1 font-mono text-xs leading-5'>
                  {primaryPrice
                    ? `$${formatBillingNumber(
                        primaryOfficialUsdPrice,
                        6,
                      )} × ${formatBillingNumber(
                        factors.forexFactor,
                        6,
                      )} × ${formatBillingNumber(
                        factors.groupFactor,
                        6,
                      )} × ${formatBillingNumber(currencyMeta.multiplier, 6)} = ${formatUnitPrice(
                        currencyMeta.symbol,
                        primaryPrice.unitPrice,
                      )}`
                    : t('选择模型后显示当前单价代入结果。')}
                </div>
              </div>
            </div>
          </details>
        </>
      ) : (
        <div className='p-4' style={CARD_STYLE}>
          <span style={{ color: 'var(--semi-color-text-2)' }}>
            {t('暂无可演示的按量计费模型')}
          </span>
        </div>
      )}
    </div>
  );

  const renderStepTwo = () => (
    <div>
      <div
        className='mb-4 text-sm'
        style={{ color: 'var(--semi-color-text-2)' }}
      >
        {t('调整本次 token 数，查看每一项费用如何代入并相加为总计。')}
      </div>

      <div
        className={`p-3 ${isMobile ? '' : 'flex items-center gap-3'}`}
        style={ACTIVE_CARD_STYLE}
      >
        <div className={isMobile ? 'w-full' : 'min-w-0 flex-1'}>
          {renderSelector()}
        </div>
        <Tag
          className={isMobile ? 'mt-3' : ''}
          color={getBillingDiscountColor(factors.compositeFactor)}
          shape='circle'
        >
          {discountText}
        </Tag>
        <div
          className={`${isMobile ? 'mt-3 text-left' : 'min-w-[150px] text-right'}`}
        >
          <div
            className='text-xs'
            style={{ color: 'var(--semi-color-text-2)' }}
          >
            {t('实时合计')}
          </div>
          <div
            className='font-mono text-2xl font-bold'
            style={{ color: 'var(--semi-color-text-0)' }}
          >
            {formatBillingMoney(prices?.symbol || '$', animatedTotal, 6)}
          </div>
        </div>
      </div>

      {prices && (
        <div
          className='my-3 rounded-xl border p-3 text-xs'
          style={{ color: 'var(--semi-color-text-2)' }}
        >
          <div className='font-semibold'>{t('完整计算公式')}</div>
          <div className='mt-1 font-mono leading-6'>
            {t(
              '实际扣费 = 各类 token 数 ÷ 1,000,000 × 对应充值单价，然后相加。',
            )}
          </div>
          <div className='mt-1 leading-5'>
            {t(
              '当前示例使用充值价格；每项单价已经包含充值汇率系数、分组倍率和展示货币换算。',
            )}
          </div>
        </div>
      )}

      <div className='flex flex-col gap-2.5'>
        {formulaRows.map((row) => (
          <div key={row.key} className='p-3' style={CARD_STYLE}>
            <div className='flex items-start justify-between gap-3'>
              <div className='min-w-0'>
                <div className='flex items-center gap-2'>
                  {row.badge && (
                    <Tag
                      className='shrink-0'
                      color='green'
                      shape='circle'
                      size='small'
                    >
                      {row.badge}
                    </Tag>
                  )}
                  <strong>{row.label}</strong>
                  <span
                    className='text-xs'
                    style={{ color: 'var(--semi-color-text-3)' }}
                  >
                    {row.description}
                  </span>
                </div>
              </div>
              <strong
                className='shrink-0 font-mono text-sm'
                style={{ color: 'var(--semi-color-success)' }}
              >
                +{row.displayedCost}
              </strong>
            </div>
            <div
              className={`mt-2 flex gap-2 ${isMobile ? 'flex-col' : 'items-center'}`}
            >
              <InputNumber
                aria-label={row.label}
                value={tokenCounts[row.key]}
                min={0}
                max={1000000000}
                precision={0}
                onChange={(value) => handleTokenChange(row.key, value)}
                style={{ width: isMobile ? '100%' : 150 }}
              />
              <span
                className='font-mono text-xs leading-5'
                style={{ color: 'var(--semi-color-text-2)' }}
              >
                {formatBillingNumber(tokenCounts[row.key], 0)} ÷ 1,000,000 ×{' '}
                {row.displayedUnitPrice} = {row.exactCost}
              </span>
            </div>
          </div>
        ))}
      </div>

      {formulaRows.length > 0 && (
        <div className='mt-3 rounded-xl border p-3' style={ACTIVE_CARD_STYLE}>
          <div className='flex items-start justify-between gap-3 text-sm'>
            <span className='font-semibold'>{t('总计等式')}</span>
            <strong
              className='font-mono text-right'
              style={{ color: 'var(--semi-color-primary)' }}
            >
              {totalEquation} ={' '}
              {formatExactMoney(prices?.symbol || '$', totalCost)}
            </strong>
          </div>
          <div
            className='mt-2 text-xs leading-5'
            style={{ color: 'var(--semi-color-text-2)' }}
          >
            {t('页面金额按 6 位小数显示；上面的等式保留更多小数，便于复核。')}
          </div>
        </div>
      )}

      <div
        className='mt-3 flex flex-wrap items-center justify-between gap-3 text-xs'
        style={{ color: 'var(--semi-color-text-3)' }}
      >
        <span>
          {prices?.cacheWrite
            ? t(
                'Claude 缓存创建按 5m / 1h 分档计费，此处按模型卡片单价估算，实际账单可能有细微差异。',
              )
            : t('计算结果按模型卡片当前单价估算。')}
        </span>
        <Button
          theme='borderless'
          type='primary'
          icon={<IconRefresh />}
          onClick={handleReplay}
        >
          {t('重新播放')}
        </Button>
      </div>
    </div>
  );

  return (
    <Modal
      visible={visible}
      onCancel={onClose}
      title={
        <div className='flex items-center gap-2'>
          <IconHelpCircle />
          <span>{step === 0 ? t('计费说明') : t('实际花费计算')}</span>
        </div>
      }
      footer={null}
      width={isMobile ? '96%' : 880}
      maskStyle={BILLING_GUIDE_MASK_STYLE}
      bodyStyle={{ overflow: 'hidden', padding: 0 }}
      style={{ maxWidth: 'calc(100vw - 16px)' }}
    >
      <div
        ref={scrollContainerRef}
        style={{
          maxHeight: isMobile ? '76vh' : '72vh',
          overflowY: 'auto',
          padding: isMobile ? '12px 16px 14px' : '16px 24px 18px',
        }}
      >
        {step === 0 ? renderStepOne() : renderStepTwo()}

        <Divider margin='18px' />
        <div className='flex items-center justify-between gap-3'>
          <div className={step > 0 ? 'min-w-[88px]' : ''}>
            {step > 0 && (
              <Button
                theme='borderless'
                type='tertiary'
                icon={<IconArrowLeft />}
                onClick={() => setStep((current) => Math.max(0, current - 1))}
              >
                {t('上一步')}
              </Button>
            )}
          </div>

          <div className='flex items-center justify-end gap-2'>
            {step === 0 && (
              <Button
                theme='borderless'
                type='tertiary'
                icon={<IconArrowRight />}
                iconPosition='right'
                onClick={() => setStep(1)}
              >
                {t('实际花费计算')}
              </Button>
            )}
            <Button theme='solid' type='primary' onClick={onClose}>
              {t('知道了')}
            </Button>
          </div>
        </div>
      </div>
    </Modal>
  );
};

export default BillingGuide;
