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

import React, { useMemo, useState } from 'react';
import { Empty, Spin, Typography } from '@douyinfe/semi-ui';
import { Gift } from 'lucide-react';
import { useTranslation } from 'react-i18next';
import { useBenefitsData } from '../../hooks/benefits/useBenefitsData';
import BenefitSummary from '../../components/benefits/BenefitSummary';
import UserVoucherCard from '../../components/benefits/UserVoucherCard';
import ClaimableActivityCard from '../../components/benefits/ClaimableActivityCard';
import UserVoucherLedgerSheet from '../../components/benefits/UserVoucherLedgerSheet';

const { Title } = Typography;

export default function Benefits() {
  const { t } = useTranslation();
  const {
    activities,
    vouchers,
    loading,
    claim,
    ledgerVoucherId,
    ledgerEntries,
    ledgerLoading,
    ledgerError,
    loadVoucherLedger,
    closeVoucherLedger,
  } = useBenefitsData();
  const [claimingId, setClaimingId] = useState(0);

  const now = useMemo(() => Math.floor(Date.now() / 1000), [vouchers]);

  const activityById = useMemo(() => {
    const map = new Map();
    activities.forEach((activity) => map.set(activity.id, activity));
    return map;
  }, [activities]);

  const ledgerVoucher = useMemo(
    () => vouchers.find((voucher) => voucher.id === ledgerVoucherId) || null,
    [vouchers, ledgerVoucherId],
  );

  const handleClaim = async (activityId) => {
    setClaimingId(activityId);
    await claim(activityId);
    setClaimingId(0);
  };

  if (loading) {
    return <Spin spinning style={{ width: '100%', padding: 48 }} />;
  }

  return (
    <main className='classic-console-page'>
      <div className='classic-console-page-container grid gap-4'>
        <div className='flex items-center gap-2 text-orange-500'>
          <Gift size={20} />
          <Title heading={3} className='!mb-0'>
            {t('活动福利')}
          </Title>
        </div>

        <BenefitSummary vouchers={vouchers} activities={activities} now={now} />

        <section className='classic-console-panel'>
          <div className='classic-console-panel-header px-4 py-3'>
            <Title heading={5} className='!mb-0'>
              {t('我的福利券')}
            </Title>
          </div>
          <div className='classic-console-panel-content'>
            {vouchers.length === 0 ? (
              <Empty description={t('暂无福利券')} />
            ) : (
              <div className='grid gap-3 md:grid-cols-2'>
                {vouchers.map((voucher) => (
                  <UserVoucherCard
                    key={voucher.id}
                    voucher={voucher}
                    activity={activityById.get(voucher.activity_id)}
                    now={now}
                    onViewLedger={(item) => loadVoucherLedger(item.id)}
                  />
                ))}
              </div>
            )}
          </div>
        </section>

        <section className='classic-console-panel'>
          <div className='classic-console-panel-header px-4 py-3'>
            <Title heading={5} className='!mb-0'>
              {t('可领取活动')}
            </Title>
          </div>
          <div className='classic-console-panel-content'>
            {activities.length === 0 ? (
              <Empty description={t('暂无活动福利')} />
            ) : (
              <div className='grid gap-3 md:grid-cols-2'>
                {activities.map((activity) => (
                  <ClaimableActivityCard
                    key={activity.id}
                    activity={activity}
                    claiming={claimingId === activity.id}
                    onClaim={handleClaim}
                  />
                ))}
              </div>
            )}
          </div>
        </section>
      </div>

      <UserVoucherLedgerSheet
        visible={ledgerVoucherId != null}
        voucher={ledgerVoucher}
        entries={ledgerEntries}
        loading={ledgerLoading}
        error={ledgerError}
        onRetry={() => ledgerVoucherId && loadVoucherLedger(ledgerVoucherId)}
        onCancel={closeVoucherLedger}
      />
    </main>
  );
}
