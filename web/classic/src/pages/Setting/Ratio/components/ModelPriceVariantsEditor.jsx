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

import React, { useEffect, useMemo, useState } from 'react';
import {
  Button,
  Card,
  Input,
  Modal,
  Switch,
  TextArea,
  Tag,
  Typography,
} from '@douyinfe/semi-ui';
import { IconDelete, IconPlus } from '@douyinfe/semi-icons';
import {
  createEmptyModelPriceVariantsState,
  formatModelPriceVariantsExpression,
  isBuiltInGrokImagineVideoModel,
  isGrokImagineVideoModel,
  parseModelPriceVariantsExpression,
} from '../../../../helpers';

const { Text } = Typography;

const normalizeFormulaKey = (value) =>
  String(value || '')
    .trim()
    .toLowerCase()
    .replaceAll('-', '_')
    .replaceAll('.', '_');

const getFormulaVariableHelp = (key, t) => {
  switch (normalizeFormulaKey(key)) {
    case 'input_base':
      return t('Input image token base.');
    case 'low_output_base':
      return t('Low quality output token base.');
    case 'medium_output_base':
      return t('Medium quality output token base.');
    case 'high_output_base':
      return t('High quality output token base.');
    case 'input_image_token_price':
      return t('Input image token price.');
    case 'output_token_price':
      return t('Output token price.');
    case 'text_input_price':
      return t('Estimated text input price.');
    case 'currency_rate':
      return t('Currency multiplier.');
    case 'input_image_unit_price':
      return t('Surcharge for each input image above the base count.');
    default:
      return '';
  }
};

const getFormulaDefaultHelp = (key, t) => {
  switch (normalizeFormulaKey(key)) {
    case 'size':
      return t('Default output size when the request omits size.');
    case 'quality':
      return t('Default output quality when the request omits quality.');
    case 'input_image_fallback_resolution':
      return t('Fallback input image size when dimensions cannot be probed.');
    default:
      return '';
  }
};

export default function ModelPriceVariantsEditor({
  model,
  isMobile,
  title,
  description,
  emptyDescription,
  restoreInheritedEnabled = true,
  onDimensionChange,
  onRuleChange,
  onAddRule,
  onDeleteRule,
  enableExtraParams = false,
  onExtraParamChange,
  onAddExtraParam,
  onDeleteExtraParam,
  enableFormula = false,
  onFormulaEnabledChange,
  onFormulaExpressionChange,
  onAddFormulaVariable,
  onDeleteFormulaVariable,
  onFormulaVariableChange,
  onAddFormulaDefault,
  onDeleteFormulaDefault,
  onFormulaDefaultChange,
  onApplyFormulaPreset,
  onRestoreInherited,
  onExpressionApply,
  t,
}) {
  const variants = model.priceVariants || createEmptyModelPriceVariantsState();
  const isGrokVideo = isGrokImagineVideoModel(model.name);
  const dimensionsEnabled =
    variants.resolutionEnabled || variants.qualityEnabled;
  const canRestoreInherited =
    restoreInheritedEnabled &&
    isBuiltInGrokImagineVideoModel(model.name) &&
    !variants.inherited &&
    !variants.restoreInherited &&
    (variants.configured || variants.rules.length > 0);
  const priceSuffix = t(model.priceUnit === 'second' ? '$/秒' : '$/次');
  const fieldCount =
    Number(variants.resolutionEnabled) + Number(variants.qualityEnabled) + 1;
  const [expressionOpen, setExpressionOpen] = useState(false);
  const [expressionText, setExpressionText] = useState('');
  const [expressionError, setExpressionError] = useState('');
  const [formulaAdvancedOpen, setFormulaAdvancedOpen] = useState(false);
  const expressionPreview = useMemo(() => {
    if (!expressionText.trim()) {
      return { draft: null, error: '' };
    }
    try {
      return {
        draft: parseModelPriceVariantsExpression(
          expressionText,
          model.name,
          variants,
          t,
        ),
        error: '',
      };
    } catch (error) {
      return {
        draft: null,
        error:
          error instanceof Error ? error.message : t('规格价格表达式不合法'),
      };
    }
  }, [expressionText, model.name, variants, t]);

  useEffect(() => {
    setExpressionText(formatModelPriceVariantsExpression(variants, model.name));
    setExpressionError('');
  }, [model.name, variants]);

  const applyExpression = () => {
    try {
      onExpressionApply?.(expressionText);
      setExpressionError('');
      setExpressionOpen(false);
    } catch (error) {
      setExpressionError(
        error instanceof Error ? error.message : t('规格价格表达式不合法'),
      );
    }
  };

  return (
    <Card
      bodyStyle={{ padding: 16 }}
      style={{
        marginTop: 16,
        marginBottom: 16,
        background: 'var(--semi-color-fill-0)',
      }}
    >
      <div className='flex items-start justify-between gap-3 mb-4'>
        <div>
          <div className='font-medium'>{title || t('规格差异计费')}</div>
          <div className='text-xs text-gray-500 mt-1'>
            {description ||
              t(
                '规则价格是对应规格的最终单价，不会与固定价格叠加；未匹配任何规则时使用固定价格兜底。',
              )}
          </div>
        </div>
        <div className='flex shrink-0 items-center gap-2'>
          {variants.inherited ? (
            <Tag color='green' shape='circle'>
              {t('继承默认')}
            </Tag>
          ) : null}
          {variants.restoreInherited ? (
            <Tag color='green' shape='circle'>
              {t('恢复内置配置')}
            </Tag>
          ) : null}
          {canRestoreInherited && !variants.inherited ? (
            <Button
              size='small'
              theme='borderless'
              onClick={onRestoreInherited}
            >
              {t('恢复内置配置')}
            </Button>
          ) : null}
        </div>
      </div>

      <div
        style={{
          display: 'grid',
          gridTemplateColumns:
            isMobile || isGrokVideo
              ? 'minmax(0, 1fr)'
              : 'repeat(2, minmax(0, 1fr))',
          gap: 12,
          marginBottom: 12,
        }}
      >
        <div className='flex items-center justify-between rounded-lg border border-gray-200 px-3 py-2'>
          <Text>{t('按分辨率区分')}</Text>
          <Switch
            size='small'
            checked={variants.resolutionEnabled}
            onChange={(checked) =>
              onDimensionChange('resolutionEnabled', checked)
            }
          />
        </div>
        {!isGrokVideo ? (
          <div className='flex items-center justify-between rounded-lg border border-gray-200 px-3 py-2'>
            <Text>{t('按质量档位区分')}</Text>
            <Switch
              size='small'
              checked={variants.qualityEnabled}
              onChange={(checked) =>
                onDimensionChange('qualityEnabled', checked)
              }
            />
          </div>
        ) : null}
      </div>

      {isGrokVideo ? (
        <div className='text-xs text-gray-500 mb-3'>
          {t('Grok 视频模型的清晰度就是分辨率，无需单独配置质量档位。')}
        </div>
      ) : null}

      <div className='mb-3'>
        <Button
          size='small'
          theme='borderless'
          onClick={() => setExpressionOpen(true)}
        >
          {t('表达式编辑')}
        </Button>
        <Modal
          title={t('规格价格表达式')}
          visible={expressionOpen}
          onCancel={() => setExpressionOpen(false)}
          onOk={applyExpression}
          okText={t('应用表达式')}
          cancelText={t('取消')}
          width={720}
        >
          <div>
            <TextArea
              value={expressionText}
              autosize={{ minRows: 10, maxRows: 18 }}
              placeholder={[
                '1024x1024 low 0.025',
                '1024x1024 medium 0.072',
                'sku_out_1024x1024_high $0.23',
              ].join('\n')}
              onChange={setExpressionText}
            />
            <div className='mt-1 text-xs text-gray-500'>
              {t(
                '每行一条规则，支持 resolution quality price、resolution price 或 AtlasCloud sku_out_* 行。',
              )}
            </div>
            {expressionError ? (
              <div className='mt-1 text-xs text-red-500'>{expressionError}</div>
            ) : null}
            {!expressionError && expressionPreview.error ? (
              <div className='mt-1 text-xs text-red-500'>
                {expressionPreview.error}
              </div>
            ) : null}
            {expressionPreview.draft ? (
              <div className='mt-3 overflow-hidden rounded-lg border border-gray-200'>
                <div className='grid grid-cols-[1fr_1fr_120px] gap-3 border-b border-gray-200 bg-gray-50 px-3 py-2 text-xs text-gray-600'>
                  <span>{t('分辨率')}</span>
                  <span>{t('质量档位')}</span>
                  <span>{t('价格')}</span>
                </div>
                <div className='max-h-56 overflow-auto'>
                  {expressionPreview.draft.rules.map((rule, index) => (
                    <div
                      key={`${rule.resolution}-${rule.quality}-${rule.price}-${index}`}
                      className='grid grid-cols-[1fr_1fr_120px] gap-3 border-b border-gray-100 px-3 py-2 text-sm last:border-b-0'
                    >
                      <span className='truncate'>
                        {expressionPreview.draft.resolutionEnabled
                          ? rule.resolution || '-'
                          : '-'}
                      </span>
                      <span className='truncate'>
                        {expressionPreview.draft.qualityEnabled
                          ? rule.quality || '-'
                          : '-'}
                      </span>
                      <span>{rule.price}</span>
                    </div>
                  ))}
                </div>
              </div>
            ) : null}
          </div>
        </Modal>
      </div>

      {dimensionsEnabled ? (
        <>
          <div className='flex items-center justify-between gap-3 mb-3'>
            <Text strong>{t('档位最终单价')}</Text>
            <Button size='small' icon={<IconPlus />} onClick={onAddRule}>
              {t('新增价格规则')}
            </Button>
          </div>

          {variants.rules.length === 0 ? (
            <div className='rounded-lg bg-gray-50 px-3 py-4 text-center text-sm text-gray-500'>
              {t('暂无价格规则，请新增至少一条规则。')}
            </div>
          ) : (
            <div className='flex flex-col gap-3'>
              {variants.rules.map((rule, index) => (
                <div
                  key={index}
                  className='rounded-lg border border-gray-200 p-3'
                >
                  <div
                    style={{
                      display: 'grid',
                      gridTemplateColumns: isMobile
                        ? 'minmax(0, 1fr)'
                        : `repeat(${fieldCount}, minmax(0, 1fr)) auto`,
                      gap: 10,
                      alignItems: 'end',
                    }}
                  >
                    {variants.resolutionEnabled ? (
                      <div>
                        <div className='text-xs text-gray-600 mb-1'>
                          {t('分辨率')}
                        </div>
                        <Input
                          value={rule.resolution}
                          placeholder={t('例如 480p')}
                          onChange={(value) =>
                            onRuleChange(index, 'resolution', value)
                          }
                        />
                      </div>
                    ) : null}
                    {variants.qualityEnabled ? (
                      <div>
                        <div className='text-xs text-gray-600 mb-1'>
                          {t('质量档位')}
                        </div>
                        <Input
                          value={rule.quality}
                          placeholder={t('例如 standard')}
                          onChange={(value) =>
                            onRuleChange(index, 'quality', value)
                          }
                        />
                      </div>
                    ) : null}
                    <div>
                      <div className='text-xs text-gray-600 mb-1'>
                        {t('最终单价')}
                      </div>
                      <Input
                        value={rule.price}
                        placeholder={t('输入档位价格')}
                        suffix={priceSuffix}
                        onChange={(value) =>
                          onRuleChange(index, 'price', value)
                        }
                      />
                    </div>
                    <Button
                      type='danger'
                      theme='borderless'
                      icon={<IconDelete />}
                      aria-label={t('删除价格规则')}
                      onClick={() => onDeleteRule(index)}
                    />
                  </div>
                </div>
              ))}
            </div>
          )}
        </>
      ) : (
        <div className='rounded-lg bg-gray-50 px-3 py-3 text-sm text-gray-500'>
          {emptyDescription ||
            t('开启至少一个规格维度后，可以配置各档位的最终单价。')}
        </div>
      )}

      {enableExtraParams ? (
        <div className='mt-4 rounded-lg border border-gray-200 p-3'>
          <div className='flex items-center justify-between gap-3 mb-2'>
            <div>
              <Text strong>{t('Extra parameter pricing')}</Text>
              <div className='text-xs text-gray-500 mt-1'>
                {t(
                  'Adds a surcharge after the route or model base price is selected.',
                )}
              </div>
            </div>
            <Button size='small' icon={<IconPlus />} onClick={onAddExtraParam}>
              {t('Add extra parameter rule')}
            </Button>
          </div>
          {variants.extraParams?.length ? (
            <div className='flex flex-col gap-3'>
              {variants.extraParams.map((rule, index) => (
                <div
                  key={index}
                  className='rounded-lg border border-gray-100 p-3'
                >
                  <div
                    style={{
                      display: 'grid',
                      gridTemplateColumns: isMobile
                        ? 'minmax(0, 1fr)'
                        : 'minmax(0, 1.3fr) minmax(0, 1fr) minmax(0, 1fr) auto',
                      gap: 10,
                      alignItems: 'end',
                    }}
                  >
                    <div>
                      <div className='text-xs text-gray-600 mb-1'>
                        {t('Parameter key')}
                      </div>
                      <Input
                        value={rule.key}
                        placeholder='input_images'
                        onChange={(value) =>
                          onExtraParamChange?.(index, 'key', value)
                        }
                      />
                    </div>
                    <div>
                      <div className='text-xs text-gray-600 mb-1'>
                        {t('Included quantity')}
                      </div>
                      <Input
                        value={rule.base}
                        placeholder='1'
                        onChange={(value) =>
                          onExtraParamChange?.(index, 'base', value)
                        }
                      />
                    </div>
                    <div>
                      <div className='text-xs text-gray-600 mb-1'>
                        {t('Extra unit price')}
                      </div>
                      <Input
                        value={rule.unitPrice}
                        placeholder='0.01'
                        suffix={priceSuffix}
                        onChange={(value) =>
                          onExtraParamChange?.(index, 'unitPrice', value)
                        }
                      />
                    </div>
                    <Button
                      type='danger'
                      theme='borderless'
                      icon={<IconDelete />}
                      aria-label={t('Delete rule')}
                      onClick={() => onDeleteExtraParam?.(index)}
                    />
                  </div>
                </div>
              ))}
            </div>
          ) : (
            <div className='rounded-lg bg-gray-50 px-3 py-3 text-sm text-gray-500'>
              {t('No extra parameter pricing rules.')}
            </div>
          )}
        </div>
      ) : null}

      {enableFormula ? (
        <div className='mt-4 rounded-lg border border-gray-200 p-3'>
          <div className='flex items-center justify-between gap-3 mb-2'>
            <div>
              <Text strong>{t('公式计费')}</Text>
              <div className='text-xs text-gray-500 mt-1'>
                {t(
                  '根据请求尺寸、输入图信息和自定义变量计算图片编辑路由的最终单价。',
                )}
              </div>
            </div>
            <Switch
              size='small'
              checked={variants.formula?.enabled === true}
              onChange={onFormulaEnabledChange}
            />
          </div>

          <div className='mb-3 rounded-lg border border-dashed border-gray-200 bg-gray-50 px-3 py-3 dark:border-gray-600 dark:bg-gray-800/40'>
            <div className='mb-3'>
              <div className='font-medium text-sm'>
                {t('Choose a pricing pattern')}
              </div>
              <div className='mt-1 text-xs text-gray-500'>
                {t(
                  'Pick the closest preset, then adjust the values shown below.',
                )}
              </div>
            </div>
            <div className='grid gap-2 md:grid-cols-3'>
              <div className='rounded-md border border-gray-200 bg-white p-3 dark:border-gray-700 dark:bg-gray-900/60'>
                <div className='font-medium text-sm'>
                  {t('AtlasCloud gpt-image-2/edit')}
                </div>
                <div className='mt-1 min-h-10 text-xs text-gray-500'>
                  {t(
                    'Use this for AtlasCloud OpenAI gpt-image-2 image editing. It follows the provider token formula.',
                  )}
                </div>
                <div className='mt-2 text-xs text-gray-600'>
                  {t(
                    'Start here. Usually only change currency_rate after applying it.',
                  )}
                </div>
                <Button
                  size='small'
                  theme='borderless'
                  style={{ width: '100%', marginTop: 12 }}
                  onClick={() => onApplyFormulaPreset?.('official')}
                >
                  {t('Apply AtlasCloud preset')}
                </Button>
              </div>
              <div className='rounded-md border border-gray-200 bg-white p-3 dark:border-gray-700 dark:bg-gray-900/60'>
                <div className='font-medium text-sm'>
                  {t('Extra input image fee')}
                </div>
                <div className='mt-1 min-h-10 text-xs text-gray-500'>
                  {t(
                    'Use when each input image after the first adds a fixed surcharge.',
                  )}
                </div>
                <div className='mt-2 text-xs text-gray-600'>
                  {t('Set input_image_unit_price to the per-image surcharge.')}
                </div>
                <Button
                  size='small'
                  theme='borderless'
                  style={{ width: '100%', marginTop: 12 }}
                  onClick={() => onApplyFormulaPreset?.('addon')}
                >
                  {t('Apply image-count preset')}
                </Button>
              </div>
              <div className='rounded-md border border-gray-200 bg-white p-3 dark:border-gray-700 dark:bg-gray-900/60'>
                <div className='font-medium text-sm'>
                  {t('Fixed edit price')}
                </div>
                <div className='mt-1 min-h-10 text-xs text-gray-500'>
                  {t(
                    'Use when the edit route should always charge the same unit price.',
                  )}
                </div>
                <div className='mt-2 text-xs text-gray-600'>
                  {t('It returns base_price and ignores size or image count.')}
                </div>
                <Button
                  size='small'
                  theme='borderless'
                  style={{ width: '100%', marginTop: 12 }}
                  onClick={() => onApplyFormulaPreset?.('fixed')}
                >
                  {t('Apply fixed-price preset')}
                </Button>
              </div>
            </div>
            <div className='mt-3 rounded-md border border-gray-200 bg-white px-3 py-2 text-xs text-gray-600 dark:border-gray-700 dark:bg-gray-900/60 dark:text-gray-300'>
              <div className='mb-1 font-medium text-gray-900 dark:text-gray-100'>
                {t('Quick start')}
              </div>
              <ul className='list-disc space-y-1 pl-4'>
                <li>
                  {t(
                    'For AtlasCloud gpt-image-2/edit, apply the AtlasCloud preset and update currency_rate.',
                  )}
                </li>
                <li>
                  {t(
                    'For models that only add a fee per extra input image, apply the image-count preset and set input_image_unit_price.',
                  )}
                </li>
                <li>
                  {t(
                    'Keep the advanced expression closed unless the provider formula changes.',
                  )}
                </li>
              </ul>
            </div>
            <div className='mt-3 text-xs text-gray-500'>
              {t(
                'Formula output uses the same unit as ModelPrice. If upstream prices are already in RMB, set currency_rate to 1; if they are in USD, set it to the exchange rate.',
              )}
            </div>
          </div>

          <div className='mb-3'>
            <Button
              size='small'
              theme='borderless'
              onClick={() => setFormulaAdvancedOpen((open) => !open)}
            >
              {t(
                formulaAdvancedOpen
                  ? 'Hide advanced formula'
                  : 'Show advanced formula',
              )}
            </Button>
            {formulaAdvancedOpen ? (
              <div className='mt-2'>
                <div className='text-xs text-gray-600 mb-1'>
                  {t('Advanced formula expression')}
                </div>
                <TextArea
                  value={variants.formula?.expression || ''}
                  autosize={{ minRows: 4, maxRows: 10 }}
                  placeholder={t(
                    'Select a template first, then edit the formula here.',
                  )}
                  onChange={onFormulaExpressionChange}
                />
                <div className='mt-1 text-xs text-gray-500'>
                  {t(
                    'You normally do not need to edit this after applying a template. Available request facts include base_price, width, height, pixels, quality, input_images, prompt_tokens_estimated, and prompt_chars.',
                  )}
                </div>
              </div>
            ) : null}
          </div>

          <div className='mb-3'>
            <div className='flex items-center justify-between gap-3 mb-2'>
              <Text strong>{t('公式变量')}</Text>
              <Button
                size='small'
                icon={<IconPlus />}
                onClick={onAddFormulaVariable}
              >
                {t('新增变量')}
              </Button>
            </div>
            <div className='mb-2 text-xs text-gray-500'>
              {t(
                'Numbers used by the formula. Template rows are the usual fields administrators need to edit.',
              )}
            </div>
            {variants.formula?.variables?.length ? (
              <div className='flex flex-col gap-2'>
                {variants.formula.variables.map((item, index) => {
                  const help = getFormulaVariableHelp(item.key, t);
                  return (
                    <div key={index}>
                      <div className='grid gap-2 sm:grid-cols-[1fr_1fr_auto]'>
                        <Input
                          value={item.key}
                          placeholder={t('Variable name')}
                          onChange={(value) =>
                            onFormulaVariableChange?.(index, 'key', value)
                          }
                        />
                        <Input
                          value={item.value}
                          placeholder={t('Number')}
                          onChange={(value) =>
                            onFormulaVariableChange?.(index, 'value', value)
                          }
                        />
                        <Button
                          type='danger'
                          theme='borderless'
                          icon={<IconDelete />}
                          aria-label={t('删除变量')}
                          onClick={() => onDeleteFormulaVariable?.(index)}
                        />
                      </div>
                      {help ? (
                        <div className='mt-1 text-xs text-gray-500'>{help}</div>
                      ) : null}
                    </div>
                  );
                })}
              </div>
            ) : (
              <div className='rounded-lg bg-gray-50 px-3 py-3 text-sm text-gray-500'>
                {t('暂无公式变量。')}
              </div>
            )}
          </div>

          <div>
            <div className='flex items-center justify-between gap-3 mb-2'>
              <Text strong>{t('公式默认值')}</Text>
              <Button
                size='small'
                icon={<IconPlus />}
                onClick={onAddFormulaDefault}
              >
                {t('新增默认值')}
              </Button>
            </div>
            <div className='mb-2 text-xs text-gray-500'>
              {t(
                'Fallback strings used only when the request omits a field, such as size, quality, or input_image_fallback_resolution.',
              )}
            </div>
            {variants.formula?.defaults?.length ? (
              <div className='flex flex-col gap-2'>
                {variants.formula.defaults.map((item, index) => {
                  const help = getFormulaDefaultHelp(item.key, t);
                  return (
                    <div key={index}>
                      <div className='grid gap-2 sm:grid-cols-[1fr_1fr_auto]'>
                        <Input
                          value={item.key}
                          placeholder={t('Default name')}
                          onChange={(value) =>
                            onFormulaDefaultChange?.(index, 'key', value)
                          }
                        />
                        <Input
                          value={item.value}
                          placeholder={t('Default value')}
                          onChange={(value) =>
                            onFormulaDefaultChange?.(index, 'value', value)
                          }
                        />
                        <Button
                          type='danger'
                          theme='borderless'
                          icon={<IconDelete />}
                          aria-label={t('删除默认值')}
                          onClick={() => onDeleteFormulaDefault?.(index)}
                        />
                      </div>
                      {help ? (
                        <div className='mt-1 text-xs text-gray-500'>{help}</div>
                      ) : null}
                    </div>
                  );
                })}
              </div>
            ) : (
              <div className='rounded-lg bg-gray-50 px-3 py-3 text-sm text-gray-500'>
                {t('暂无公式默认值。')}
              </div>
            )}
          </div>
        </div>
      ) : null}
    </Card>
  );
}
