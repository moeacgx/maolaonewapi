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

import React, { useCallback, useEffect, useState } from 'react';
import {
  Banner,
  Button,
  Card,
  InputNumber,
  Radio,
  RadioGroup,
  Select,
  Space,
  Spin,
  Switch,
  Toast,
  Typography,
} from '@douyinfe/semi-ui';
import { useTranslation } from 'react-i18next';
import SettingsSensitiveWords from '../Setting/Operation/SettingsSensitiveWords';
import {
  getSecurityAuditBuiltinPolicy,
  getSecurityAuditBuiltinPolicyChannels,
  getSecurityAuditBuiltinPolicyGroups,
  updateSecurityAuditBuiltinPolicy,
} from './api';

const { Text } = Typography;

const getErrorMessage = (error, fallback) =>
  error?.response?.data?.message || error?.message || fallback;

const TARGET_ALL = 'all';
const SELECT_ALL_CHANNELS = '__select_all_channels__';
const SELECT_ALL_GROUPS = '__select_all_groups__';
const TARGET_CHANNELS = 'channels';
const TARGET_GROUPS = 'groups';

const getChannelLabel = (channel) => {
  const name = String(channel?.name || '').trim();
  const label = name ? `${name} #${channel.id}` : `#${channel?.id}`;
  const tag = String(channel?.tag || '').trim();
  return tag ? `${label} · ${tag}` : label;
};

const arraysEqual = (left, right) =>
  JSON.stringify(left || []) === JSON.stringify(right || []);

const BuiltinPolicyTab = ({ onSaved }) => {
  const { t } = useTranslation();
  const [baseline, setBaseline] = useState(null);
  const [draft, setDraft] = useState(null);
  const [loading, setLoading] = useState(true);
  const [saving, setSaving] = useState(false);
  const [loadError, setLoadError] = useState('');
  const [channels, setChannels] = useState([]);
  const [channelsLoading, setChannelsLoading] = useState(false);
  const [channelsError, setChannelsError] = useState(false);
  const [groups, setGroups] = useState([]);
  const [groupsLoading, setGroupsLoading] = useState(false);
  const [groupsError, setGroupsError] = useState(false);

  const loadPolicy = useCallback(async () => {
    setLoading(true);
    setLoadError('');
    try {
      const policy = await getSecurityAuditBuiltinPolicy();
      setBaseline(policy);
      setDraft(policy);
    } catch (error) {
      setLoadError(getErrorMessage(error, t('内置安全策略加载失败')));
    } finally {
      setLoading(false);
    }
  }, [t]);

  useEffect(() => {
    void loadPolicy();
  }, [loadPolicy]);

  const loadChannels = useCallback(async () => {
    setChannelsLoading(true);
    setChannelsError(false);
    try {
      const nextChannels = await getSecurityAuditBuiltinPolicyChannels();
      setChannels(
        [...nextChannels]
          .filter((channel) => Number.isInteger(channel?.id) && channel.id > 0)
          .sort((left, right) => {
            const labelCompare = getChannelLabel(left).localeCompare(
              getChannelLabel(right),
            );
            return labelCompare === 0 ? left.id - right.id : labelCompare;
          }),
      );
    } catch {
      setChannelsError(true);
    } finally {
      setChannelsLoading(false);
    }
  }, []);

  const loadGroups = useCallback(async () => {
    setGroupsLoading(true);
    setGroupsError(false);
    try {
      const nextGroups = await getSecurityAuditBuiltinPolicyGroups();
      setGroups(
        [...nextGroups]
          .filter((group) => String(group?.code || '').trim())
          .sort((left, right) =>
            String(left.name || left.code).localeCompare(
              String(right.name || right.code),
            ),
          ),
      );
    } catch {
      setGroupsError(true);
    } finally {
      setGroupsLoading(false);
    }
  }, []);

  useEffect(() => {
    void loadChannels();
    void loadGroups();
  }, [loadChannels, loadGroups]);

  const switchesDirty = Boolean(
    draft &&
      baseline &&
      (draft.upstream_policy_enabled !== baseline.upstream_policy_enabled ||
        draft.sensitive_word_audit_enabled !==
          baseline.sensitive_word_audit_enabled ||
        draft.cyber_session_block_enabled !==
          baseline.cyber_session_block_enabled ||
        draft.cyber_session_block_ttl_seconds !==
          baseline.cyber_session_block_ttl_seconds ||
        draft.cyber_policy_auto_ban_enabled !==
          baseline.cyber_policy_auto_ban_enabled ||
        !arraysEqual(
          draft.cyber_policy_auto_ban_exempt_group_codes,
          baseline.cyber_policy_auto_ban_exempt_group_codes,
        ) ||
        draft.cyber_policy_ban_threshold !==
          baseline.cyber_policy_ban_threshold ||
        draft.cyber_policy_violation_window_hours !==
          baseline.cyber_policy_violation_window_hours ||
        draft.upstream_policy_target_type !==
          baseline.upstream_policy_target_type ||
        !arraysEqual(
          draft.upstream_policy_channel_ids,
          baseline.upstream_policy_channel_ids,
        ) ||
        !arraysEqual(
          draft.upstream_policy_group_codes,
          baseline.upstream_policy_group_codes,
        )),
  );

  const applySavedPolicy = (policy) => {
    setBaseline(policy);
    setDraft(policy);
    onSaved?.(policy);
    Toast.success({ content: t('内置安全策略已保存') });
  };

  const savePolicy = async (values) => {
    if (!draft) return;
    if (
      draft.upstream_policy_target_type === TARGET_CHANNELS &&
      !draft.upstream_policy_channel_ids?.length
    ) {
      Toast.warning({ content: t('请至少选择一个官方风控生效渠道') });
      return;
    }
    if (
      draft.upstream_policy_target_type === TARGET_GROUPS &&
      !draft.upstream_policy_group_codes?.length
    ) {
      Toast.warning({ content: t('请至少选择一个官方风控生效分组') });
      return;
    }
    setSaving(true);
    try {
      const saved = await updateSecurityAuditBuiltinPolicy({
        ...draft,
        check_sensitive_enabled: values.CheckSensitiveEnabled,
        check_sensitive_on_prompt_enabled: values.CheckSensitiveOnPromptEnabled,
        sensitive_rules: values.SensitiveRules,
        sensitive_rule_channel_ids: values.SensitiveRuleChannelIds,
      });
      applySavedPolicy(saved);
    } catch (error) {
      if (error?.response?.status === 409) {
        Toast.error({
          content: t('配置已被其他管理员更新，已重新加载最新版本'),
        });
        await loadPolicy();
      } else {
        Toast.error({
          content: getErrorMessage(error, t('内置安全策略保存失败')),
        });
      }
    } finally {
      setSaving(false);
    }
  };

  if (loadError) {
    return (
      <Banner
        type='danger'
        closeIcon={null}
        description={
          <Space wrap>
            <span>{loadError}</span>
            <Button size='small' onClick={() => void loadPolicy()}>
              {t('重试')}
            </Button>
          </Space>
        }
      />
    );
  }

  return (
    <Spin spinning={loading && !draft}>
      {draft ? (
        <div className='space-y-4'>
          <Banner
            type='info'
            closeIcon={null}
            description={t(
              'Guard 节点不是必需项：屏蔽词在本机运行，上游返回精确的 cyber_policy 错误码时也会被识别。',
            )}
          />

          <Card title={t('内置安全策略')} bodyStyle={{ padding: 16 }}>
            <Text type='tertiary' size='small'>
              {t('选择哪些无需 Guard 节点的检测结果写入统一审计事件。')}
            </Text>
            <div className='mt-4 grid grid-cols-1 gap-3 lg:grid-cols-2'>
              <div className='rounded-lg border border-[var(--semi-color-border)] p-4 lg:col-span-2'>
                <Space align='start' className='w-full'>
                  <Switch
                    checked={draft.upstream_policy_enabled}
                    onChange={(enabled) =>
                      setDraft((current) => ({
                        ...current,
                        upstream_policy_enabled: enabled,
                        cyber_session_block_enabled:
                          enabled && current.cyber_session_block_enabled,
                        cyber_policy_auto_ban_enabled:
                          enabled && current.cyber_policy_auto_ban_enabled,
                      }))
                    }
                  />
                  <div className='min-w-0 flex-1'>
                    <Text strong>{t('识别上游安全策略事件')}</Text>
                    <Text type='tertiary' size='small' className='mt-1 block'>
                      {t(
                        '记录 HTTP、流式响应和 Realtime 上游返回的精确 cyber_policy 拒绝。',
                      )}
                    </Text>
                    <div className='mt-4'>
                      <Text strong size='small' className='block'>
                        {t('官方风控作用范围')}
                      </Text>
                      <Text type='tertiary' size='small' className='mt-1 block'>
                        {t(
                          '选择哪些渠道返回的 cyber_policy 事件写入安全审计。',
                        )}
                      </Text>
                      <RadioGroup
                        className='mt-3'
                        value={draft.upstream_policy_target_type}
                        onChange={(event) =>
                          setDraft((current) => ({
                            ...current,
                            upstream_policy_target_type: event.target.value,
                          }))
                        }
                      >
                        <Radio value={TARGET_ALL}>{t('全部渠道')}</Radio>
                        <Radio value={TARGET_CHANNELS}>{t('指定渠道')}</Radio>
                        <Radio value={TARGET_GROUPS}>{t('指定分组')}</Radio>
                      </RadioGroup>

                      {draft.upstream_policy_target_type === TARGET_ALL ? (
                        <Text
                          type='tertiary'
                          size='small'
                          className='mt-3 block'
                        >
                          {t('该策略对所有渠道生效')}
                        </Text>
                      ) : draft.upstream_policy_target_type ===
                        TARGET_CHANNELS ? (
                        <div className='mt-3'>
                          <Select
                            multiple
                            filter
                            maxTagCount={1}
                            ellipsisTrigger
                            showRestTagsPopover
                            loading={channelsLoading}
                            disabled={channelsError}
                            value={draft.upstream_policy_channel_ids || []}
                            placeholder={t('指定渠道')}
                            emptyContent={t('暂无渠道')}
                            className='w-full'
                            onChange={(value) =>
                              setDraft((current) => ({
                                ...current,
                                upstream_policy_channel_ids:
                                  Array.isArray(value) &&
                                  value.includes(SELECT_ALL_CHANNELS)
                                    ? channels.map((channel) => channel.id)
                                    : Array.isArray(value)
                                      ? value
                                      : [],
                              }))
                            }
                          >
                            <Select.Option value={SELECT_ALL_CHANNELS}>
                              {t('全部渠道')}
                            </Select.Option>
                            {channels.map((channel) => (
                              <Select.Option
                                key={channel.id}
                                value={channel.id}
                              >
                                {getChannelLabel(channel)}
                              </Select.Option>
                            ))}
                            {(draft.upstream_policy_channel_ids || [])
                              .filter(
                                (id) =>
                                  !channels.some(
                                    (channel) => channel.id === Number(id),
                                  ),
                              )
                              .map((id) => (
                                <Select.Option key={`missing-${id}`} value={id}>
                                  {t('失效渠道')} #{id}
                                </Select.Option>
                              ))}
                          </Select>
                          {channelsError ? (
                            <Space wrap className='mt-2'>
                              <Text type='danger' size='small'>
                                {t('获取渠道列表失败')}
                              </Text>
                              <Button
                                type='tertiary'
                                theme='borderless'
                                size='small'
                                onClick={() => void loadChannels()}
                              >
                                {t('重试')}
                              </Button>
                            </Space>
                          ) : null}
                        </div>
                      ) : (
                        <div className='mt-3'>
                          <Select
                            multiple
                            filter
                            maxTagCount={1}
                            ellipsisTrigger
                            showRestTagsPopover
                            loading={groupsLoading}
                            disabled={groupsError}
                            value={draft.upstream_policy_group_codes || []}
                            placeholder={t('指定分组')}
                            emptyContent={t('暂无分组')}
                            className='w-full'
                            onChange={(value) =>
                              setDraft((current) => ({
                                ...current,
                                upstream_policy_group_codes:
                                  Array.isArray(value) &&
                                  value.includes(SELECT_ALL_GROUPS)
                                    ? groups.map((group) => group.code)
                                    : Array.isArray(value)
                                      ? value
                                      : [],
                              }))
                            }
                          >
                            <Select.Option value={SELECT_ALL_GROUPS}>
                              {t('全部分组')}
                            </Select.Option>
                            {groups.map((group) => (
                              <Select.Option
                                key={group.code}
                                value={group.code}
                              >
                                {group.name || group.code} ({group.code})
                              </Select.Option>
                            ))}
                            {(draft.upstream_policy_group_codes || [])
                              .filter(
                                (code) =>
                                  !groups.some(
                                    (group) => group.code === String(code),
                                  ),
                              )
                              .map((code) => (
                                <Select.Option
                                  key={`missing-${code}`}
                                  value={code}
                                >
                                  {t('失效分组')}: {code}
                                </Select.Option>
                              ))}
                          </Select>
                          {groupsError ? (
                            <Space wrap className='mt-2'>
                              <Text type='danger' size='small'>
                                {t('获取分组列表失败')}
                              </Text>
                              <Button
                                type='tertiary'
                                theme='borderless'
                                size='small'
                                onClick={() => void loadGroups()}
                              >
                                {t('重试')}
                              </Button>
                            </Space>
                          ) : null}
                        </div>
                      )}
                    </div>
                  </div>
                </Space>
              </div>
              <div className='rounded-lg border border-[var(--semi-color-border)] p-4'>
                <Space align='start'>
                  <Switch
                    checked={draft.sensitive_word_audit_enabled}
                    onChange={(enabled) =>
                      setDraft((current) => ({
                        ...current,
                        sensitive_word_audit_enabled: enabled,
                      }))
                    }
                  />
                  <div>
                    <Text strong>{t('记录屏蔽词事件')}</Text>
                    <Text type='tertiary' size='small' className='mt-1 block'>
                      {t(
                        '请求、返回或 Realtime 命中屏蔽词时，去重后写入统一审计事件。',
                      )}
                    </Text>
                  </div>
                </Space>
              </div>
              <div className='rounded-lg border border-[var(--semi-color-border)] p-4 lg:col-span-2'>
                <Space align='start'>
                  <Switch
                    checked={draft.cyber_session_block_enabled}
                    onChange={(enabled) =>
                      setDraft((current) => ({
                        ...current,
                        cyber_session_block_enabled: enabled,
                        upstream_policy_enabled:
                          enabled || current.upstream_policy_enabled,
                      }))
                    }
                  />
                  <div className='min-w-0 flex-1'>
                    <Text strong>{t('上游 cyber_policy 后屏蔽当前会话')}</Text>
                    <Text type='tertiary' size='small' className='mt-1 block'>
                      {t(
                        '开启后，命中上游 cyber_policy 的显式会话会在 TTL 内由本地直接拒绝；同一 API Key 下其他会话不受影响。',
                      )}
                    </Text>
                    <div className='mt-4'>
                      <Text size='small' className='mb-2 block'>
                        {t('会话屏蔽 TTL（秒）')}
                      </Text>
                      <InputNumber
                        className='w-full'
                        min={1}
                        max={31536000}
                        precision={0}
                        value={draft.cyber_session_block_ttl_seconds}
                        disabled={!draft.cyber_session_block_enabled}
                        onChange={(value) => {
                          const parsed = Number(value);
                          if (Number.isInteger(parsed)) {
                            setDraft((current) => ({
                              ...current,
                              cyber_session_block_ttl_seconds: parsed,
                            }));
                          }
                        }}
                      />
                      <Text type='tertiary' size='small' className='mt-1 block'>
                        {t(
                          '仅使用会话请求头或 prompt_cache_key；没有显式会话标识的请求不会被本地会话屏蔽。',
                        )}
                      </Text>
                    </div>
                  </div>
                </Space>
              </div>
              <div className='rounded-lg border border-[var(--semi-color-border)] p-4 lg:col-span-2'>
                <Space align='start'>
                  <Switch
                    checked={draft.cyber_policy_auto_ban_enabled}
                    onChange={(enabled) =>
                      setDraft((current) => ({
                        ...current,
                        cyber_policy_auto_ban_enabled: enabled,
                        upstream_policy_enabled:
                          enabled || current.upstream_policy_enabled,
                      }))
                    }
                  />
                  <div className='min-w-0 flex-1'>
                    <Text strong>
                      {t('cyber_policy 达到阈值后自动禁用用户')}
                    </Text>
                    <Text type='tertiary' size='small' className='mt-1 block'>
                      {t('仅处置普通用户，管理员和 Root 永不自动禁用。')}
                    </Text>
                    <div className='mt-4 grid grid-cols-1 gap-4 sm:grid-cols-2'>
                      <div>
                        <Text size='small' className='mb-2 block'>
                          {t('违规次数阈值')}
                        </Text>
                        <InputNumber
                          className='w-full'
                          min={1}
                          max={1000000}
                          precision={0}
                          value={draft.cyber_policy_ban_threshold}
                          disabled={!draft.cyber_policy_auto_ban_enabled}
                          onChange={(value) => {
                            const parsed = Number(value);
                            if (Number.isInteger(parsed)) {
                              setDraft((current) => ({
                                ...current,
                                cyber_policy_ban_threshold: parsed,
                              }));
                            }
                          }}
                        />
                        <Text
                          type='tertiary'
                          size='small'
                          className='mt-1 block'
                        >
                          {t('设为 1 表示首次命中即禁用。')}
                        </Text>
                      </div>
                      <div>
                        <Text size='small' className='mb-2 block'>
                          {t('滚动窗口（小时）')}
                        </Text>
                        <InputNumber
                          className='w-full'
                          min={1}
                          max={87600}
                          precision={0}
                          value={draft.cyber_policy_violation_window_hours}
                          disabled={!draft.cyber_policy_auto_ban_enabled}
                          onChange={(value) => {
                            const parsed = Number(value);
                            if (Number.isInteger(parsed)) {
                              setDraft((current) => ({
                                ...current,
                                cyber_policy_violation_window_hours: parsed,
                              }));
                            }
                          }}
                        />
                        <Text
                          type='tertiary'
                          size='small'
                          className='mt-1 block'
                        >
                          {t('只统计该时间范围内精确的 cyber_policy 事件。')}
                        </Text>
                      </div>
                    </div>
                    <div className='mt-4'>
                      <Text size='small' className='mb-2 block'>
                        {t('自动禁用分组白名单')}
                      </Text>
                      <Select
                        multiple
                        filter
                        maxTagCount={1}
                        ellipsisTrigger
                        showRestTagsPopover
                        loading={groupsLoading}
                        disabled={
                          !draft.cyber_policy_auto_ban_enabled || groupsError
                        }
                        value={
                          draft.cyber_policy_auto_ban_exempt_group_codes || []
                        }
                        placeholder={t('选择免于自动禁用的分组')}
                        emptyContent={t('暂无分组')}
                        className='w-full'
                        onChange={(value) =>
                          setDraft((current) => ({
                            ...current,
                            cyber_policy_auto_ban_exempt_group_codes:
                              Array.isArray(value) ? value : [],
                          }))
                        }
                      >
                        {groups.map((group) => (
                          <Select.Option key={group.code} value={group.code}>
                            {group.name || group.code} ({group.code})
                          </Select.Option>
                        ))}
                        {(draft.cyber_policy_auto_ban_exempt_group_codes || [])
                          .filter(
                            (code) =>
                              !groups.some(
                                (group) => group.code === String(code),
                              ),
                          )
                          .map((code) => (
                            <Select.Option
                              key={`missing-exempt-${code}`}
                              value={code}
                            >
                              {t('失效分组')}: {code}
                            </Select.Option>
                          ))}
                      </Select>
                      <Text type='tertiary' size='small' className='mt-1 block'>
                        {t(
                          '选中的业务分组不参与 cyber_policy 次数累计，也不会触发自动禁用；其他分组仍按阈值处置。',
                        )}
                      </Text>
                      <Text type='warning' size='small' className='mt-1 block'>
                        {t(
                          '白名单只能缩小自动禁用范围；如需除白名单外所有分组生效，请将上方官方风控作用范围设为“全部渠道”。',
                        )}
                      </Text>
                      {groupsError ? (
                        <Space wrap className='mt-2'>
                          <Text type='danger' size='small'>
                            {t('获取分组列表失败')}
                          </Text>
                          <Button
                            type='tertiary'
                            theme='borderless'
                            size='small'
                            onClick={() => void loadGroups()}
                          >
                            {t('重试')}
                          </Button>
                        </Space>
                      ) : null}
                    </div>
                  </div>
                </Space>
              </div>
            </div>
          </Card>

          {draft.uses_legacy_sensitive_words ? (
            <Banner
              type='warning'
              closeIcon={null}
              description={t(
                '检测到旧版屏蔽词：页面已将其显示为结构化阻断规则，保存时会保留原始数据并启用结构化规则。',
              )}
            />
          ) : null}

          <Card
            title={t('屏蔽词过滤')}
            headerExtraContent={
              <Text type='tertiary' size='small'>
                {t('原系统设置中的规则已迁移到这里，继续使用同一份配置。')}
              </Text>
            }
            bodyStyle={{ padding: 16 }}
          >
            <SettingsSensitiveWords
              options={{
                CheckSensitiveEnabled: draft.check_sensitive_enabled,
                CheckSensitiveOnPromptEnabled:
                  draft.check_sensitive_on_prompt_enabled,
                SensitiveWords: draft.sensitive_words,
                SensitiveRules: draft.sensitive_rules,
                SensitiveRuleChannelIds: draft.sensitive_rule_channel_ids,
              }}
              externalDirty={switchesDirty}
              saving={saving}
              onSave={savePolicy}
              onResetExternal={() => setDraft(baseline)}
            />
          </Card>
        </div>
      ) : (
        <div className='min-h-72' />
      )}
    </Spin>
  );
};

export default BuiltinPolicyTab;
