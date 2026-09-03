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

import { useCallback, useEffect, useState } from 'react';
import { API, showError, showSuccess } from '../../helpers';
import { useTranslation } from 'react-i18next';

export function useBenefitsData() {
  const { t } = useTranslation();
  const [activities, setActivities] = useState([]);
  const [vouchers, setVouchers] = useState([]);
  const [loading, setLoading] = useState(true);

  const load = useCallback(async () => {
    setLoading(true);
    try {
      const [activityResponse, voucherResponse] = await Promise.all([
        API.get('/api/benefit/activities'),
        API.get('/api/benefit/vouchers'),
      ]);
      if (activityResponse.data?.success) {
        setActivities(activityResponse.data.data || []);
      } else {
        showError(activityResponse.data?.message || t('无法加载活动福利'));
      }
      if (voucherResponse.data?.success) {
        setVouchers(voucherResponse.data.data || []);
      } else {
        showError(voucherResponse.data?.message || t('无法加载福利券'));
      }
    } catch (error) {
      showError(error?.response?.data?.message || t('无法加载活动福利'));
    } finally {
      setLoading(false);
    }
  }, [t]);

  useEffect(() => {
    load();
  }, [load]);

  const claim = async (activityId) => {
    try {
      const response = await API.post(
        `/api/benefit/activities/${activityId}/claim`,
      );
      if (!response.data?.success) {
        showError(response.data?.message || t('无法领取福利'));
        return false;
      }
      showSuccess(t('福利领取成功'));
      await load();
      return true;
    } catch (error) {
      showError(error?.response?.data?.message || t('无法领取福利'));
      return false;
    }
  };

  return {
    activities,
    vouchers,
    loading,
    refresh: load,
    claim,
  };
}

export async function fetchAdminBenefitActivities() {
  const response = await API.get(
    '/api/benefit/admin/activities?p=1&page_size=100',
  );
  return response.data?.data?.items || [];
}
