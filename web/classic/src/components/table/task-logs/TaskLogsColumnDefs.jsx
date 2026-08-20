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
import { Progress, Tag, Tooltip, Typography } from '@douyinfe/semi-ui';
import {
  Music,
  FileText,
  HelpCircle,
  CheckCircle,
  Pause,
  Clock,
  Play,
  XCircle,
  Loader,
  List,
  Image,
  Sparkles,
} from 'lucide-react';
import {
  TASK_ACTION_FIRST_TAIL_GENERATE,
  TASK_ACTION_GENERATE,
  TASK_ACTION_REFERENCE_GENERATE,
  TASK_ACTION_TEXT_GENERATE,
  TASK_ACTION_REMIX_GENERATE,
} from '../../../constants/common.constant';
import { CHANNEL_OPTIONS } from '../../../constants/channel.constants';
import { stringToColor } from '../../../helpers/render';
import { Avatar, Space } from '@douyinfe/semi-ui';

const colors = [
  'amber',
  'blue',
  'cyan',
  'green',
  'grey',
  'indigo',
  'light-blue',
  'lime',
  'orange',
  'pink',
  'purple',
  'red',
  'teal',
  'violet',
  'yellow',
];

const buildVideoProxyUrl = (taskId) => {
  if (typeof taskId !== 'string' || taskId.trim() === '') {
    return '';
  }
  return `/v1/videos/${encodeURIComponent(taskId.trim())}/content`;
};

// Render functions
const renderTimestamp = (timestampInSeconds) => {
  const date = new Date(timestampInSeconds * 1000); // 从秒转换为毫秒

  const year = date.getFullYear(); // 获取年份
  const month = ('0' + (date.getMonth() + 1)).slice(-2); // 获取月份，从0开始需要+1，并保证两位数
  const day = ('0' + date.getDate()).slice(-2); // 获取日期，并保证两位数
  const hours = ('0' + date.getHours()).slice(-2); // 获取小时，并保证两位数
  const minutes = ('0' + date.getMinutes()).slice(-2); // 获取分钟，并保证两位数
  const seconds = ('0' + date.getSeconds()).slice(-2); // 获取秒钟，并保证两位数

  return `${year}-${month}-${day} ${hours}:${minutes}:${seconds}`; // 格式化输出
};

function renderDuration(submit_time, finishTime) {
  if (!submit_time || !finishTime) return 'N/A';
  const durationSec = finishTime - submit_time;
  const color = durationSec > 60 ? 'red' : 'green';

  // 返回带有样式的颜色标签
  return (
    <Tag color={color} shape='circle'>
      {durationSec} s
    </Tag>
  );
}

const renderType = (type, t) => {
  switch (type) {
    case 'images/generations':
      return (
        <Tag color='purple' shape='circle' prefixIcon={<Image size={14} />}>
          {t('绘图')}
        </Tag>
      );
    case 'images/edits':
      return (
        <Tag color='violet' shape='circle' prefixIcon={<Image size={14} />}>
          {t('编辑')}
        </Tag>
      );
    case 'MUSIC':
      return (
        <Tag color='grey' shape='circle' prefixIcon={<Music size={14} />}>
          {t('生成音乐')}
        </Tag>
      );
    case 'LYRICS':
      return (
        <Tag color='pink' shape='circle' prefixIcon={<FileText size={14} />}>
          {t('生成歌词')}
        </Tag>
      );
    case TASK_ACTION_GENERATE:
      return (
        <Tag color='blue' shape='circle' prefixIcon={<Sparkles size={14} />}>
          {t('图生视频')}
        </Tag>
      );
    case TASK_ACTION_TEXT_GENERATE:
      return (
        <Tag color='blue' shape='circle' prefixIcon={<Sparkles size={14} />}>
          {t('文生视频')}
        </Tag>
      );
    case TASK_ACTION_FIRST_TAIL_GENERATE:
      return (
        <Tag color='blue' shape='circle' prefixIcon={<Sparkles size={14} />}>
          {t('首尾生视频')}
        </Tag>
      );
    case TASK_ACTION_REFERENCE_GENERATE:
      return (
        <Tag color='blue' shape='circle' prefixIcon={<Sparkles size={14} />}>
          {t('参照生视频')}
        </Tag>
      );
    case TASK_ACTION_REMIX_GENERATE:
      return (
        <Tag color='blue' shape='circle' prefixIcon={<Sparkles size={14} />}>
          {t('视频Remix')}
        </Tag>
      );
    default:
      return (
        <Tag color='white' shape='circle' prefixIcon={<HelpCircle size={14} />}>
          {t('未知')}
        </Tag>
      );
  }
};

const atlasCloudProviderFromTask = (record) => {
  const explicit = record?.display_platform;
  if (typeof explicit === 'string' && explicit.trim() !== '') {
    return explicit.trim();
  }
  const properties = record?.properties || {};
  const modelName = String(
    properties.origin_model_name || properties.upstream_model_name || '',
  ).toLowerCase();
  if (modelName.includes('grok') || modelName.startsWith('xai/')) {
    return 'xAI';
  }
  if (
    modelName.startsWith('openai/') ||
    modelName.includes('gpt-image') ||
    modelName.includes('sora')
  ) {
    return 'OpenAI';
  }
  return '';
};

const renderPlatform = (platform, t, record) => {
  if (String(platform) === '58') {
    const provider = atlasCloudProviderFromTask(record);
    if (provider) {
      return (
        <Tag color={provider === 'xAI' ? 'cyan' : 'green'} shape='circle'>
          {provider}
        </Tag>
      );
    }
  }
  let option = CHANNEL_OPTIONS.find(
    (opt) => String(opt.value) === String(platform),
  );
  if (option) {
    return (
      <Tag color={option.color} shape='circle'>
        {option.label}
      </Tag>
    );
  }
  switch (platform) {
    case 'image':
      return (
        <Tag color='violet' shape='circle'>
          Image API
        </Tag>
      );
    case 'canvas_image':
      return (
        <Tag color='purple' shape='circle'>
          Canvas
        </Tag>
      );
    case 'suno':
      return (
        <Tag color='green' shape='circle'>
          Suno
        </Tag>
      );
    default:
      return (
        <Tag color='white' shape='circle'>
          {t('未知')}
        </Tag>
      );
  }
};

const renderStatus = (type, t) => {
  switch (type) {
    case 'SUCCESS':
      return (
        <Tag
          color='green'
          shape='circle'
          prefixIcon={<CheckCircle size={14} />}
        >
          {t('成功')}
        </Tag>
      );
    case 'NOT_START':
      return (
        <Tag color='grey' shape='circle' prefixIcon={<Pause size={14} />}>
          {t('未启动')}
        </Tag>
      );
    case 'SUBMITTED':
      return (
        <Tag color='yellow' shape='circle' prefixIcon={<Clock size={14} />}>
          {t('队列中')}
        </Tag>
      );
    case 'IN_PROGRESS':
      return (
        <Tag color='blue' shape='circle' prefixIcon={<Play size={14} />}>
          {t('执行中')}
        </Tag>
      );
    case 'FAILURE':
      return (
        <Tag color='red' shape='circle' prefixIcon={<XCircle size={14} />}>
          {t('失败')}
        </Tag>
      );
    case 'QUEUED':
      return (
        <Tag color='orange' shape='circle' prefixIcon={<List size={14} />}>
          {t('排队中')}
        </Tag>
      );
    case 'UNKNOWN':
      return (
        <Tag color='white' shape='circle' prefixIcon={<HelpCircle size={14} />}>
          {t('未知')}
        </Tag>
      );
    case '':
      return (
        <Tag color='grey' shape='circle' prefixIcon={<Loader size={14} />}>
          {t('正在提交')}
        </Tag>
      );
    default:
      return (
        <Tag color='white' shape='circle' prefixIcon={<HelpCircle size={14} />}>
          {t('未知')}
        </Tag>
      );
  }
};

const getTaskModelInfo = (record) => {
  const properties = record?.properties || {};
  const requestModel = properties.origin_model_name || '';
  const actualModel = properties.upstream_model_name || '';
  return {
    requestModel: requestModel || actualModel || '',
    actualModel,
  };
};

const renderModel = (record, t) => {
  const { requestModel, actualModel } = getTaskModelInfo(record);
  if (!requestModel && !actualModel) {
    return <Typography.Text type='tertiary'>-</Typography.Text>;
  }
  return (
    <Space spacing={4} wrap>
      {requestModel ? (
        <Tag color='blue' shape='circle'>
          {requestModel}
        </Tag>
      ) : null}
      {actualModel && actualModel !== requestModel ? (
        <Tooltip content={t('实际模型')}>
          <Tag color='purple' shape='circle'>
            {actualModel}
          </Tag>
        </Tooltip>
      ) : null}
    </Space>
  );
};

const renderGroup = (record) => {
  const group = String(record?.group || '').trim();
  const displayName = String(record?.group_name || group).trim();
  if (!displayName) {
    return <Typography.Text type='tertiary'>-</Typography.Text>;
  }
  return (
    <Tag color={colors[group.length % colors.length]} shape='circle'>
      {displayName}
    </Tag>
  );
};

export const getTaskLogsColumns = ({
  t,
  COLUMN_KEYS,
  copyText,
  openContentModal,
  isAdminUser,
  openVideoModal,
  openAudioModal,
  openImagePreview,
  showUserInfoFunc,
  showChannelInfoFunc,
}) => {
  return [
    {
      key: COLUMN_KEYS.SUBMIT_TIME,
      title: t('提交时间'),
      dataIndex: 'submit_time',
      render: (text, record, index) => {
        return <div>{text ? renderTimestamp(text) : '-'}</div>;
      },
    },
    {
      key: COLUMN_KEYS.FINISH_TIME,
      title: t('结束时间'),
      dataIndex: 'finish_time',
      render: (text, record, index) => {
        return <div>{text ? renderTimestamp(text) : '-'}</div>;
      },
    },
    {
      key: COLUMN_KEYS.DURATION,
      title: t('花费时间'),
      dataIndex: 'finish_time',
      render: (finish, record) => {
        return <>{finish ? renderDuration(record.submit_time, finish) : '-'}</>;
      },
    },
    {
      key: COLUMN_KEYS.CHANNEL,
      title: t('渠道'),
      dataIndex: 'channel_id',
      render: (text, record, index) => {
        if (!isAdminUser) {
          return <></>;
        }
        return (
          <div>
            <Tag
              color={colors[parseInt(text || 0) % colors.length]}
              size='large'
              shape='circle'
              onClick={(event) => {
                event.stopPropagation();
                if (showChannelInfoFunc) {
                  showChannelInfoFunc(text);
                  return;
                }
                copyText(text);
              }}
            >
              {text || '-'}
            </Tag>
          </div>
        );
      },
    },
    {
      key: COLUMN_KEYS.USERNAME,
      title: t('用户'),
      dataIndex: 'username',
      render: (userId, record, index) => {
        if (!isAdminUser) {
          return <></>;
        }
        const displayText = String(record.username || userId || '?');
        return (
          <Space>
            <Avatar size='extra-small' color={stringToColor(displayText)}>
              {displayText.slice(0, 1)}
            </Avatar>
            <Typography.Text
              link
              onClick={(event) => {
                event.stopPropagation();
                showUserInfoFunc?.(record.user_id);
              }}
            >
              {displayText}
            </Typography.Text>
          </Space>
        );
      },
    },
    {
      key: COLUMN_KEYS.PLATFORM,
      title: t('平台'),
      dataIndex: 'platform',
      render: (text, record, index) => {
        return <div>{renderPlatform(text, t, record)}</div>;
      },
    },
    {
      key: COLUMN_KEYS.TYPE,
      title: t('类型'),
      dataIndex: 'action',
      render: (text, record, index) => {
        return <div>{renderType(text, t)}</div>;
      },
    },
    {
      key: COLUMN_KEYS.MODEL,
      title: t('模型'),
      dataIndex: 'properties',
      render: (_, record) => renderModel(record, t),
    },
    {
      key: COLUMN_KEYS.GROUP,
      title: t('分组'),
      dataIndex: 'group',
      render: (_, record) => renderGroup(record),
    },
    {
      key: COLUMN_KEYS.TASK_ID,
      title: t('任务ID'),
      dataIndex: 'task_id',
      render: (text, record, index) => {
        return (
          <Tooltip content={t('点击行展开任务详情')}>
            <Typography.Text ellipsis={{ showTooltip: true }}>
              <div>{text}</div>
            </Typography.Text>
          </Tooltip>
        );
      },
    },
    {
      key: COLUMN_KEYS.TASK_STATUS,
      title: t('任务状态'),
      dataIndex: 'status',
      render: (text, record, index) => {
        return <div>{renderStatus(text, t)}</div>;
      },
    },
    {
      key: COLUMN_KEYS.PROGRESS,
      title: t('进度'),
      dataIndex: 'progress',
      render: (text, record, index) => {
        return (
          <div>
            {isNaN(text?.replace('%', '')) ? (
              text || '-'
            ) : (
              <Progress
                stroke={
                  record.status === 'FAILURE'
                    ? 'var(--semi-color-warning)'
                    : null
                }
                percent={text ? parseInt(text.replace('%', '')) : 0}
                showInfo={true}
                aria-label='task progress'
                style={{ minWidth: '160px' }}
              />
            )}
          </div>
        );
      },
    },
    {
      key: COLUMN_KEYS.FAIL_REASON,
      title: t('详情'),
      dataIndex: 'fail_reason',
      fixed: 'right',
      render: (text, record, index) => {
        // Suno audio preview
        const isSunoSuccess =
          record.platform === 'suno' &&
          record.status === 'SUCCESS' &&
          Array.isArray(record.data) &&
          record.data.some((c) => c.audio_url);
        if (isSunoSuccess) {
          return (
            <a
              href='#'
              onClick={(e) => {
                e.preventDefault();
                e.stopPropagation();
                openAudioModal(record.data);
              }}
            >
              {t('点击预览音乐')}
            </a>
          );
        }

        // 视频预览：优先使用 result_url，兼容旧数据 fail_reason 中的 URL
        const isVideoTask =
          record.action === TASK_ACTION_GENERATE ||
          record.action === TASK_ACTION_TEXT_GENERATE ||
          record.action === TASK_ACTION_FIRST_TAIL_GENERATE ||
          record.action === TASK_ACTION_REFERENCE_GENERATE ||
          record.action === TASK_ACTION_REMIX_GENERATE;
        const isSuccess = record.status === 'SUCCESS';
        const imageUrls = Array.isArray(record.image_urls)
          ? record.image_urls.filter(
              (url) => typeof url === 'string' && url.trim() !== '',
            )
          : [];
        if (isSuccess && imageUrls.length > 0) {
          return (
            <a
              href='#'
              onClick={(e) => {
                e.preventDefault();
                e.stopPropagation();
                openImagePreview(imageUrls, record.task_id);
              }}
            >
              {t('查看图片')}
            </a>
          );
        }
        if (isSuccess && record.result_expired) {
          return (
            <Typography.Text type='tertiary'>{t('已过期')}</Typography.Text>
          );
        }

        const resultUrl = record.result_url;
        const hasResultUrl =
          typeof resultUrl === 'string' && /^https?:\/\//.test(resultUrl);
        const videoUrl = buildVideoProxyUrl(record.task_id) || resultUrl;
        if (isSuccess && isVideoTask && (videoUrl || hasResultUrl)) {
          return (
            <a
              href='#'
              onClick={(e) => {
                e.preventDefault();
                e.stopPropagation();
                openVideoModal(videoUrl);
              }}
            >
              {t('点击预览视频')}
            </a>
          );
        }
        if (!text) {
          return t('无');
        }
        return (
          <Typography.Text
            ellipsis={{ showTooltip: true }}
            style={{ width: 100 }}
            onClick={(event) => {
              event.stopPropagation();
              openContentModal(text);
            }}
          >
            {text}
          </Typography.Text>
        );
      },
    },
  ];
};
