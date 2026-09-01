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

import { useEffect, useState } from 'react';
import { Modal } from '@douyinfe/semi-ui';
import { API, showError, showSuccess } from '../../helpers';
import { ITEMS_PER_PAGE } from '../../constants';
import { useTranslation } from 'react-i18next';

export const PROMO_CODE_STATUS = {
  ENABLED: 1,
  DISABLED: 2,
  USED: 3,
};

// /api/promo_code/batch and /api/promo_code/invalid both respond with
// { deleted_ids: number[], skipped: { id, reason }[] }. The deleted count is
// always deleted_ids.length — never the requested id count — so a real 0
// (e.g. everything was skipped) renders as 0 instead of a false "N deleted".
const extractBatchDeleteResult = (data) => ({
  deletedIds: data?.deleted_ids || [],
  skipped: data?.skipped || [],
});

export const usePromoCodesData = () => {
  const { t } = useTranslation();
  const [promoCodes, setPromoCodes] = useState([]);
  const [plans, setPlans] = useState([]);
  const [loading, setLoading] = useState(false);
  const [searching, setSearching] = useState(false);
  const [activePage, setActivePage] = useState(1);
  const [pageSize, setPageSize] = useState(ITEMS_PER_PAGE);
  const [total, setTotal] = useState(0);
  const [searchKeyword, setSearchKeyword] = useState('');
  const [selectedKeys, setSelectedKeys] = useState([]);

  const loadPlans = async () => {
    try {
      const res = await API.get('/api/subscription/admin/plans');
      if (res?.data?.success) {
        setPlans((res.data.data || []).map((item) => item.plan || item));
      } else {
        setPlans([]);
      }
    } catch (error) {
      setPlans([]);
    }
  };

  const loadPromoCodes = async (page = activePage, size = pageSize) => {
    setLoading(true);
    try {
      const res = await API.get(`/api/promo_code/?p=${page}&page_size=${size}`);
      if (res?.data?.success) {
        const items = res.data.data?.items || [];
        // A page emptied out from under us (e.g. batch delete on the last
        // page) — fall back one page instead of rendering a blank table.
        if (items.length === 0 && page > 1) {
          await loadPromoCodes(page - 1, size);
          return;
        }
        setPromoCodes(items);
        setTotal(res.data.data?.total || 0);
        setActivePage(res.data.data?.page || page);
      } else {
        showError(res?.data?.message || t('优惠码加载失败'));
      }
    } catch (error) {
      showError(error.message || t('优惠码加载失败'));
    } finally {
      setLoading(false);
    }
  };

  const searchPromoCodes = async (
    keyword = searchKeyword,
    page = 1,
    size = pageSize,
  ) => {
    setSelectedKeys([]);
    const trimmed = String(keyword || '').trim();
    setSearchKeyword(trimmed);
    if (!trimmed) {
      await loadPromoCodes(page, size);
      return;
    }
    setSearching(true);
    try {
      const res = await API.get(
        `/api/promo_code/search?keyword=${encodeURIComponent(
          trimmed,
        )}&p=${page}&page_size=${size}`,
      );
      if (res?.data?.success) {
        const items = res.data.data?.items || [];
        // A page emptied out from under us (e.g. batch delete on the last
        // page of search results) — fall back one page.
        if (items.length === 0 && page > 1) {
          await searchPromoCodes(keyword, page - 1, size);
          return;
        }
        setPromoCodes(items);
        setTotal(res.data.data?.total || 0);
        setActivePage(res.data.data?.page || page);
      } else {
        showError(res?.data?.message || t('优惠码搜索失败'));
      }
    } catch (error) {
      showError(error.message || t('优惠码搜索失败'));
    } finally {
      setSearching(false);
    }
  };

  const refresh = async (page = activePage) => {
    if (searchKeyword) {
      await searchPromoCodes(searchKeyword, page, pageSize);
    } else {
      await loadPromoCodes(page, pageSize);
    }
  };

  const savePromoCode = async (payload) => {
    const res = payload.id
      ? await API.put('/api/promo_code/', payload)
      : await API.post('/api/promo_code/', payload);
    if (res?.data?.success) {
      showSuccess(payload.id ? t('优惠码更新成功') : t('优惠码创建成功'));
      await refresh();
      return true;
    }
    showError(res?.data?.message || t('保存失败'));
    return false;
  };

  const updatePromoCodeStatus = async (record, status) => {
    const res = await API.put('/api/promo_code/?status_only=true', {
      id: record.id,
      status,
    });
    if (res?.data?.success) {
      showSuccess(t('操作成功完成！'));
      await refresh();
    } else {
      showError(res?.data?.message || t('操作失败'));
    }
  };

  const deletePromoCode = async (record) => {
    setLoading(true);
    try {
      const res = await API.delete(`/api/promo_code/${record.id}`);
      if (res?.data?.success) {
        showSuccess(t('删除成功'));
        setSelectedKeys((prev) => prev.filter((id) => id !== record.id));
        await refresh();
      } else {
        showError(res?.data?.message || t('删除失败'));
      }
    } catch (error) {
      showError(
        error?.response?.data?.message || error.message || t('删除失败'),
      );
    } finally {
      setLoading(false);
    }
  };

  // 批量删除当前页选中的优惠码。selectedKeys holds ids, not row objects.
  const batchDeletePromoCodes = async () => {
    if (selectedKeys.length === 0) {
      showError(t('请至少选择一个优惠码！'));
      return;
    }
    setLoading(true);
    try {
      const ids = selectedKeys.filter(Boolean);
      const res = await API.delete('/api/promo_code/batch', { data: { ids } });
      const { success, message, data } = res.data || {};
      if (!success) {
        showError(message || t('批量删除失败'));
        return;
      }
      setSelectedKeys([]);
      const { deletedIds, skipped } = extractBatchDeleteResult(data);
      if (skipped.length === 0) {
        showSuccess(
          t('已删除 {{count}} 条优惠码', { count: deletedIds.length }),
        );
      } else {
        Modal.warning({
          title: t('Deleted {{deleted}}, skipped {{skipped}} promo codes', {
            deleted: deletedIds.length,
            skipped: skipped.length,
          }),
          content: (
            <ul className='list-disc pl-4 space-y-1'>
              {skipped.map((item) => (
                <li key={item.id}>{`#${item.id}: ${item.reason}`}</li>
              ))}
            </ul>
          ),
        });
      }
      await refresh();
    } catch (error) {
      showError(
        error?.response?.data?.message || error.message || t('批量删除失败'),
      );
    } finally {
      setLoading(false);
    }
  };

  // Clears out disabled/exhausted/expired promo codes in one request,
  // mirroring redemption's /api/redemption/invalid cleanup.
  const deleteInvalidPromoCodes = async () => {
    setLoading(true);
    try {
      const res = await API.delete('/api/promo_code/invalid');
      const { success, message, data } = res.data || {};
      if (!success) {
        showError(message || t('Failed to clear invalid promo codes'));
        return;
      }
      setSelectedKeys([]);
      const { deletedIds, skipped } = extractBatchDeleteResult(data);
      if (skipped.length === 0) {
        showSuccess(
          t('Cleared {{count}} invalid promo codes', {
            count: deletedIds.length,
          }),
        );
      } else {
        Modal.warning({
          title: t(
            'Cleared {{deleted}}, skipped {{skipped}} invalid promo codes',
            { deleted: deletedIds.length, skipped: skipped.length },
          ),
          content: (
            <ul className='list-disc pl-4 space-y-1'>
              {skipped.map((item) => (
                <li key={item.id}>{`#${item.id}: ${item.reason}`}</li>
              ))}
            </ul>
          ),
        });
      }
      await refresh();
    } catch (error) {
      showError(
        error?.response?.data?.message ||
          error.message ||
          t('Failed to clear invalid promo codes'),
      );
    } finally {
      setLoading(false);
    }
  };

  const handlePageChange = (page) => {
    setSelectedKeys([]);
    setActivePage(page);
    if (searchKeyword) {
      searchPromoCodes(searchKeyword, page, pageSize);
    } else {
      loadPromoCodes(page, pageSize);
    }
  };

  const handlePageSizeChange = (size) => {
    setSelectedKeys([]);
    setPageSize(size);
    setActivePage(1);
    if (searchKeyword) {
      searchPromoCodes(searchKeyword, 1, size);
    } else {
      loadPromoCodes(1, size);
    }
  };

  useEffect(() => {
    loadPlans();
    loadPromoCodes(1, pageSize);
  }, []);

  return {
    t,
    promoCodes,
    plans,
    loading,
    searching,
    activePage,
    pageSize,
    total,
    searchKeyword,
    setSearchKeyword,
    loadPromoCodes,
    searchPromoCodes,
    refresh,
    savePromoCode,
    updatePromoCodeStatus,
    deletePromoCode,
    batchDeletePromoCodes,
    deleteInvalidPromoCodes,
    selectedKeys,
    setSelectedKeys,
    handlePageChange,
    handlePageSizeChange,
  };
};
