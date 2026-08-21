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

import { useCallback, useEffect, useRef } from 'react';
import { useTranslation } from 'react-i18next';
import {
  API,
  processModelsData,
  processGroupsData,
  showError,
} from '../../helpers';
import { API_ENDPOINTS } from '../../constants/playground.constants';

export const useDataLoader = (
  userState,
  inputs,
  handleInputChange,
  setModels,
  setGroups,
) => {
  const { t } = useTranslation();
  const inputsRef = useRef(inputs);
  const modelsRequestIdRef = useRef(0);
  const groupsRequestIdRef = useRef(0);
  const modelsAbortControllerRef = useRef(null);
  const groupsAbortControllerRef = useRef(null);

  // 在渲染阶段同步引用，避免分组切换后旧响应在新 effect 启动前写回状态。
  inputsRef.current = inputs;

  const loadModels = useCallback(async () => {
    const requestedGroup = String(inputsRef.current.group ?? '').trim();
    const requestId = ++modelsRequestIdRef.current;
    modelsAbortControllerRef.current?.abort();
    const abortController = new AbortController();
    modelsAbortControllerRef.current = abortController;

    // 切换分组时先移除上一分组的模型，避免短暂显示错误列表。
    setModels([]);

    try {
      const res = await API.get(API_ENDPOINTS.USER_MODELS, {
        params: { group: requestedGroup },
        skipErrorHandler: true,
        signal: abortController.signal,
      });

      if (
        abortController.signal.aborted ||
        requestId !== modelsRequestIdRef.current ||
        requestedGroup !== String(inputsRef.current.group ?? '').trim()
      ) {
        return;
      }

      const { success, message, data } = res.data || {};

      if (success) {
        const { modelOptions, selectedModel } = processModelsData(
          data,
          inputsRef.current.model,
        );
        setModels(modelOptions);

        if (selectedModel !== inputsRef.current.model) {
          handleInputChange('model', selectedModel);
        }
      } else {
        showError(t(message || '加载模型失败'));
      }
    } catch (error) {
      if (
        abortController.signal.aborted ||
        requestId !== modelsRequestIdRef.current
      ) {
        return;
      }
      showError(t('加载模型失败'));
    } finally {
      if (modelsAbortControllerRef.current === abortController) {
        modelsAbortControllerRef.current = null;
      }
    }
  }, [handleInputChange, setModels, t]);

  const loadGroups = useCallback(async () => {
    const requestId = ++groupsRequestIdRef.current;
    groupsAbortControllerRef.current?.abort();
    const abortController = new AbortController();
    groupsAbortControllerRef.current = abortController;

    try {
      const res = await API.get(API_ENDPOINTS.USER_GROUPS, {
        skipErrorHandler: true,
        signal: abortController.signal,
      });

      if (
        abortController.signal.aborted ||
        requestId !== groupsRequestIdRef.current
      ) {
        return;
      }

      const { success, message, data } = res.data || {};

      if (success) {
        let storedUserGroup = '';
        try {
          storedUserGroup = JSON.parse(
            localStorage.getItem('user') || '{}',
          )?.group;
        } catch {}
        const userGroup = userState?.user?.group || storedUserGroup;
        const currentGroup = String(inputsRef.current.group ?? '').trim();
        const groupOptions = processGroupsData(data, currentGroup, userGroup);
        setGroups(groupOptions);

        const hasCurrentGroup = groupOptions.some(
          (option) => option.value === currentGroup,
        );
        if (!hasCurrentGroup) {
          handleInputChange('group', groupOptions[0]?.value || '');
        }
      } else {
        showError(t(message || '加载分组失败'));
      }
    } catch (error) {
      if (
        abortController.signal.aborted ||
        requestId !== groupsRequestIdRef.current
      ) {
        return;
      }
      showError(t('加载分组失败'));
    } finally {
      if (groupsAbortControllerRef.current === abortController) {
        groupsAbortControllerRef.current = null;
      }
    }
  }, [userState?.user?.group, handleInputChange, setGroups, t]);

  // 自动加载数据
  useEffect(() => {
    if (userState?.user) {
      loadGroups();
    }
  }, [userState?.user, loadGroups]);

  useEffect(() => {
    if (userState?.user) {
      if (!String(inputs.group ?? '').trim()) {
        setModels([]);
        return;
      }
      loadModels();
    }
  }, [userState?.user, inputs.group, loadModels, setModels]);

  useEffect(() => {
    return () => {
      modelsAbortControllerRef.current?.abort();
      groupsAbortControllerRef.current?.abort();
      modelsRequestIdRef.current += 1;
      groupsRequestIdRef.current += 1;
    };
  }, []);

  return {
    loadModels,
    loadGroups,
  };
};
