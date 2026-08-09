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

import { useState, useEffect, useRef } from 'react';
import { useTranslation } from 'react-i18next';
import { Modal } from '@douyinfe/semi-ui';
import {
  API,
  copy,
  isAdmin,
  showError,
  showSuccess,
  timestamp2string,
} from '../../helpers';
import { ITEMS_PER_PAGE } from '../../constants';
import { useTableCompactMode } from '../common/useTableCompactMode';

export const useTaskLogsData = () => {
  const { t } = useTranslation();

  // Define column keys for selection
  const COLUMN_KEYS = {
    SUBMIT_TIME: 'submit_time',
    FINISH_TIME: 'finish_time',
    DURATION: 'duration',
    CHANNEL: 'channel',
    USERNAME: 'username',
    PLATFORM: 'platform',
    TYPE: 'type',
    MODEL: 'model',
    GROUP: 'group',
    TASK_ID: 'task_id',
    TASK_STATUS: 'task_status',
    PROGRESS: 'progress',
    FAIL_REASON: 'fail_reason',
    RESULT_URL: 'result_url',
  };

  // Basic state
  const [logs, setLogs] = useState([]);
  const [loading, setLoading] = useState(false);
  const [activePage, setActivePage] = useState(1);
  const [logCount, setLogCount] = useState(0);
  const [pageSize, setPageSize] = useState(ITEMS_PER_PAGE);

  // User and admin
  const isAdminUser = isAdmin();
  // Role-specific storage key to prevent different roles from overwriting each other
  const STORAGE_KEY = isAdminUser
    ? 'task-logs-table-columns-admin'
    : 'task-logs-table-columns-user';

  // Modal state
  const [isModalOpen, setIsModalOpen] = useState(false);
  const [modalContent, setModalContent] = useState('');

  // 新增：视频预览弹窗状态
  const [isVideoModalOpen, setIsVideoModalOpen] = useState(false);
  const [videoUrl, setVideoUrl] = useState('');

  // Audio preview modal state
  const [isAudioModalOpen, setIsAudioModalOpen] = useState(false);
  const [audioClips, setAudioClips] = useState([]);

  // 图片任务预览状态
  const [isImagePreviewOpen, setIsImagePreviewOpen] = useState(false);
  const [imagePreviewUrls, setImagePreviewUrls] = useState([]);
  const [imagePreviewTaskId, setImagePreviewTaskId] = useState('');
  const imagePreviewObjectUrls = useRef([]);
  const imagePreviewMounted = useRef(true);
  const imagePreviewRequest = useRef(null);
  const imagePreviewLoading = useRef(false);

  // User info modal state
  const [showUserInfo, setShowUserInfoModal] = useState(false);
  const [userInfoData, setUserInfoData] = useState(null);

  // Channel info modal state
  const [showChannelInfo, setShowChannelInfoModal] = useState(false);
  const [channelInfoData, setChannelInfoData] = useState(null);

  // Expanded row details
  const [expandData, setExpandData] = useState({});

  // Form state
  const [formApi, setFormApi] = useState(null);
  let now = new Date();
  let zeroNow = new Date(now.getFullYear(), now.getMonth(), now.getDate());

  const formInitValues = {
    channel_id: '',
    task_id: '',
    dateRange: [
      timestamp2string(zeroNow.getTime() / 1000),
      timestamp2string(now.getTime() / 1000 + 3600),
    ],
  };

  // Column visibility state
  const [visibleColumns, setVisibleColumns] = useState({});
  const [showColumnSelector, setShowColumnSelector] = useState(false);

  // Compact mode
  const [compactMode, setCompactMode] = useTableCompactMode('taskLogs');

  // Load saved column preferences from localStorage
  useEffect(() => {
    const savedColumns = localStorage.getItem(STORAGE_KEY);
    if (savedColumns) {
      try {
        const parsed = JSON.parse(savedColumns);
        const defaults = getDefaultColumnVisibility();
        const validKeys = new Set(Object.values(COLUMN_KEYS));
        const persisted =
          parsed && typeof parsed === 'object' && !Array.isArray(parsed)
            ? Object.fromEntries(
                Object.entries(parsed).filter(
                  ([key, value]) =>
                    validKeys.has(key) && typeof value === 'boolean',
                ),
              )
            : {};
        const merged = { ...defaults, ...persisted };

        // For non-admin users, force-hide admin-only columns (does not touch admin settings)
        if (!isAdminUser) {
          merged[COLUMN_KEYS.CHANNEL] = false;
          merged[COLUMN_KEYS.USERNAME] = false;
        }
        setVisibleColumns(merged);
      } catch (e) {
        console.error('Failed to parse saved column preferences', e);
        initDefaultColumns();
      }
    } else {
      initDefaultColumns();
    }
  }, []);

  // Get default column visibility based on user role
  const getDefaultColumnVisibility = () => {
    return {
      [COLUMN_KEYS.SUBMIT_TIME]: true,
      [COLUMN_KEYS.FINISH_TIME]: true,
      [COLUMN_KEYS.DURATION]: true,
      [COLUMN_KEYS.CHANNEL]: isAdminUser,
      [COLUMN_KEYS.USERNAME]: isAdminUser,
      [COLUMN_KEYS.PLATFORM]: true,
      [COLUMN_KEYS.TYPE]: true,
      [COLUMN_KEYS.TASK_ID]: true,
      [COLUMN_KEYS.MODEL]: true,
      [COLUMN_KEYS.GROUP]: true,
      [COLUMN_KEYS.TASK_STATUS]: true,
      [COLUMN_KEYS.PROGRESS]: true,
      [COLUMN_KEYS.FAIL_REASON]: true,
      [COLUMN_KEYS.RESULT_URL]: true,
    };
  };

  // Initialize default column visibility
  const initDefaultColumns = () => {
    const defaults = getDefaultColumnVisibility();
    setVisibleColumns(defaults);
    localStorage.setItem(STORAGE_KEY, JSON.stringify(defaults));
  };

  // Handle column visibility change
  const handleColumnVisibilityChange = (columnKey, checked) => {
    const updatedColumns = { ...visibleColumns, [columnKey]: checked };
    setVisibleColumns(updatedColumns);
  };

  // Handle "Select All" checkbox
  const handleSelectAll = (checked) => {
    const allKeys = Object.keys(COLUMN_KEYS).map((key) => COLUMN_KEYS[key]);
    const updatedColumns = {};

    allKeys.forEach((key) => {
      if (
        (key === COLUMN_KEYS.CHANNEL || key === COLUMN_KEYS.USERNAME) &&
        !isAdminUser
      ) {
        updatedColumns[key] = false;
      } else {
        updatedColumns[key] = checked;
      }
    });

    setVisibleColumns(updatedColumns);
  };

  // Persist column settings to the role-specific STORAGE_KEY
  useEffect(() => {
    if (Object.keys(visibleColumns).length > 0) {
      localStorage.setItem(STORAGE_KEY, JSON.stringify(visibleColumns));
    }
  }, [visibleColumns]);

  // Get form values helper function
  const getFormValues = () => {
    const formValues = formApi ? formApi.getValues() : {};

    // 处理时间范围
    let start_timestamp = timestamp2string(zeroNow.getTime() / 1000);
    let end_timestamp = timestamp2string(now.getTime() / 1000 + 3600);

    if (
      formValues.dateRange &&
      Array.isArray(formValues.dateRange) &&
      formValues.dateRange.length === 2
    ) {
      start_timestamp = formValues.dateRange[0];
      end_timestamp = formValues.dateRange[1];
    }

    return {
      channel_id: formValues.channel_id || '',
      task_id: formValues.task_id || '',
      start_timestamp,
      end_timestamp,
    };
  };

  const getTaskModelDisplay = (task) => {
    const properties = task?.properties || {};
    return (
      properties.origin_model_name || properties.upstream_model_name || '-'
    );
  };

  const getTaskActualModelDisplay = (task) => {
    const properties = task?.properties || {};
    return properties.upstream_model_name || '-';
  };

  const buildTaskExpandData = (task) => {
    const result = [
      { key: t('任务ID'), value: task.task_id || '-' },
      { key: t('模型'), value: getTaskModelDisplay(task) },
      { key: t('请求模型'), value: task.properties?.origin_model_name || '-' },
      { key: t('实际模型'), value: getTaskActualModelDisplay(task) },
      { key: t('分组'), value: task.group || '-' },
      { key: t('渠道'), value: task.channel_id ? `#${task.channel_id}` : '-' },
      { key: t('用户'), value: task.username || `#${task.user_id || '-'}` },
      { key: t('进度'), value: task.progress || '-' },
    ];
    if (task.result_url) {
      result.push({ key: t('结果地址'), value: task.result_url });
    }
    if (task.fail_reason) {
      result.push({ key: t('详情'), value: task.fail_reason });
    }
    return result;
  };

  // Enrich logs data
  const enrichLogs = (items) => {
    return items.map((log) => ({
      ...log,
      timestamp2string: timestamp2string(log.created_at),
      key: '' + log.id,
    }));
  };

  const buildExpandData = (items) => {
    return items.reduce((acc, task) => {
      acc[task.key] = buildTaskExpandData(task);
      return acc;
    }, {});
  };

  // Sync page data
  const syncPageData = (payload) => {
    const items = enrichLogs(payload.items || []);
    setLogs(items);
    setExpandData(buildExpandData(items));
    setLogCount(payload.total || 0);
    setActivePage(payload.page || 1);
    setPageSize(payload.page_size || pageSize);
  };

  // Load logs function
  const loadLogs = async (page = 1, size = pageSize) => {
    setLoading(true);
    const { channel_id, task_id, start_timestamp, end_timestamp } =
      getFormValues();
    let localStartTimestamp = parseInt(Date.parse(start_timestamp) / 1000);
    let localEndTimestamp = parseInt(Date.parse(end_timestamp) / 1000);
    let url = isAdminUser
      ? `/api/task/?p=${page}&page_size=${size}&channel_id=${channel_id}&task_id=${task_id}&start_timestamp=${localStartTimestamp}&end_timestamp=${localEndTimestamp}`
      : `/api/task/self?p=${page}&page_size=${size}&task_id=${task_id}&start_timestamp=${localStartTimestamp}&end_timestamp=${localEndTimestamp}`;
    const res = await API.get(url);
    const { success, message, data } = res.data;
    if (success) {
      syncPageData(data);
    } else {
      showError(message);
    }
    setLoading(false);
  };

  // Page handlers
  const handlePageChange = (page) => {
    loadLogs(page, pageSize).then();
  };

  const handlePageSizeChange = async (size) => {
    localStorage.setItem('task-page-size', size + '');
    await loadLogs(1, size);
  };

  // Refresh function
  const refresh = async () => {
    await loadLogs(1, pageSize);
  };

  // Copy text function
  const copyText = async (text) => {
    if (await copy(text)) {
      showSuccess(t('已复制：') + text);
    } else {
      Modal.error({ title: t('无法复制到剪贴板，请手动复制'), content: text });
    }
  };

  // Modal handlers
  const openContentModal = (content) => {
    setModalContent(content);
    setIsModalOpen(true);
  };

  // 新增：打开视频预览弹窗
  const openVideoModal = (url) => {
    setVideoUrl(url);
    setIsVideoModalOpen(true);
  };

  const openAudioModal = (clips) => {
    setAudioClips(clips);
    setIsAudioModalOpen(true);
  };

  const clearImagePreviewObjectUrls = () => {
    imagePreviewObjectUrls.current.forEach((url) => URL.revokeObjectURL(url));
    imagePreviewObjectUrls.current = [];
  };

  useEffect(() => {
    imagePreviewMounted.current = true;
    return () => {
      imagePreviewMounted.current = false;
      imagePreviewRequest.current?.abort();
      clearImagePreviewObjectUrls();
    };
  }, []);

  const handleImagePreviewVisibleChange = (visible) => {
    setIsImagePreviewOpen(visible);
    if (!visible) {
      imagePreviewRequest.current?.abort();
      clearImagePreviewObjectUrls();
      setImagePreviewUrls([]);
      setImagePreviewTaskId('');
    }
  };

  const openImagePreview = async (urls, taskId) => {
    const validUrls = Array.isArray(urls)
      ? urls.filter((url) => typeof url === 'string' && url.trim() !== '')
      : [];
    if (validUrls.length === 0) {
      return;
    }

    if (imagePreviewLoading.current) {
      return;
    }

    const previewUrl = validUrls[0];
    const abortController = new AbortController();
    imagePreviewRequest.current?.abort();
    imagePreviewRequest.current = abortController;
    imagePreviewLoading.current = true;
    clearImagePreviewObjectUrls();

    try {
      let resolvedUrl = previewUrl;
      if (previewUrl.startsWith('/api/task/')) {
        const response = await API.get(previewUrl, {
          responseType: 'blob',
          disableDuplicate: true,
          skipErrorHandler: true,
          signal: abortController.signal,
        });
        if (!imagePreviewMounted.current || abortController.signal.aborted) {
          return;
        }
        resolvedUrl = URL.createObjectURL(response.data);
        imagePreviewObjectUrls.current.push(resolvedUrl);
      }

      if (!imagePreviewMounted.current || abortController.signal.aborted) {
        return;
      }
      setImagePreviewUrls([resolvedUrl]);
      setImagePreviewTaskId(taskId || '');
      setIsImagePreviewOpen(true);
    } catch {
      if (!abortController.signal.aborted) {
        clearImagePreviewObjectUrls();
        showError(t('加载失败'));
      }
    } finally {
      if (imagePreviewRequest.current === abortController) {
        imagePreviewRequest.current = null;
        imagePreviewLoading.current = false;
      }
    }
  };

  // User info function
  const showUserInfoFunc = async (userId) => {
    if (!isAdminUser) {
      return;
    }
    const res = await API.get(`/api/user/${userId}`);
    const { success, message, data } = res.data;
    if (success) {
      setUserInfoData(data);
      setShowUserInfoModal(true);
    } else {
      showError(message);
    }
  };

  const showChannelInfoFunc = async (channelId) => {
    if (!isAdminUser || !channelId) {
      return;
    }
    const res = await API.get(`/api/channel/${channelId}`);
    const { success, message, data } = res.data;
    if (success) {
      setChannelInfoData(data);
      setShowChannelInfoModal(true);
    } else {
      showError(message);
    }
  };

  const hasExpandableRows = () => {
    return Object.values(expandData).some((items) => items.length > 0);
  };

  // Initialize data
  useEffect(() => {
    const localPageSize =
      parseInt(localStorage.getItem('task-page-size')) || ITEMS_PER_PAGE;
    setPageSize(localPageSize);
    loadLogs(1, localPageSize).then();
  }, []);

  return {
    // Basic state
    logs,
    loading,
    activePage,
    logCount,
    pageSize,
    isAdminUser,

    // Modal state
    isModalOpen,
    setIsModalOpen,
    modalContent,

    // 新增：视频弹窗状态
    isVideoModalOpen,
    setIsVideoModalOpen,
    videoUrl,

    // Audio preview modal
    isAudioModalOpen,
    setIsAudioModalOpen,
    audioClips,

    // 图片任务预览
    isImagePreviewOpen,
    setIsImagePreviewOpen: handleImagePreviewVisibleChange,
    imagePreviewUrls,
    imagePreviewTaskId,

    // Form state
    formApi,
    setFormApi,
    formInitValues,
    getFormValues,

    // Column visibility
    visibleColumns,
    showColumnSelector,
    setShowColumnSelector,
    handleColumnVisibilityChange,
    handleSelectAll,
    initDefaultColumns,
    COLUMN_KEYS,

    // Compact mode
    compactMode,
    setCompactMode,

    // User info modal
    showUserInfo,
    setShowUserInfoModal,
    userInfoData,
    showUserInfoFunc,

    // Channel info modal
    showChannelInfo,
    setShowChannelInfoModal,
    channelInfoData,
    showChannelInfoFunc,

    // Expanded row details
    expandData,
    hasExpandableRows,

    // Functions
    loadLogs,
    handlePageChange,
    handlePageSizeChange,
    refresh,
    copyText,
    openContentModal,
    openVideoModal,
    openAudioModal,
    openImagePreview,
    enrichLogs,
    syncPageData,

    // Translation
    t,
  };
};
