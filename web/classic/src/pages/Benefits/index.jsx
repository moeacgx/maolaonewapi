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
import { Button, Card, Empty, Spin, Tag, Typography } from '@douyinfe/semi-ui';
import { Gift, TicketCheck } from 'lucide-react';
import { useTranslation } from 'react-i18next';
import { renderQuota } from '../../helpers';
import { useBenefitsData } from '../../hooks/benefits/useBenefitsData';

const { Title, Text } = Typography;

export default function Benefits() {
  const { t } = useTranslation();
  const { activities, vouchers, loading, claim } = useBenefitsData();

  if (loading) {
    return <Spin spinning style={{ width: '100%', padding: 48 }} />;
  }

  return (
    <main className='classic-console-page'>
      <div className='classic-console-page-container'>
        <div className='mb-4 flex items-center gap-2 text-orange-500'>
          <Gift size={20} />
          <Title heading={3} className='!mb-0'>
            {t('活动福利')}
          </Title>
        </div>

        <section className='mb-6'>
          <Title heading={5}>{t('我的福利券')}</Title>
          {vouchers.length === 0 ? (
            <Empty description={t('暂无福利券')} />
          ) : (
            <div className='grid gap-3 md:grid-cols-2'>
              {vouchers.map((voucher) => {
                const activity = activities.find(
                  (item) => item.id === voucher.activity_id,
                );
                return (
                  <Card key={voucher.id} bodyStyle={{ padding: 16 }}>
                    <div className='flex items-center justify-between gap-3'>
                      <span className='inline-flex items-center gap-2 font-semibold'>
                        <TicketCheck size={16} />
                        {renderQuota(voucher.remaining_quota)}
                      </span>
                      <Tag>{t(voucher.status)}</Tag>
                    </div>
                    <Text type='tertiary'>
                      {t('失效时间')}:{' '}
                      {new Date(voucher.expires_at * 1000).toLocaleString(
                        undefined,
                        { timeZone: 'Asia/Shanghai' },
                      )}
                    </Text>
                    <Text type='tertiary'>
                      {t('原始额度')}: {voucher.original_quota}
                    </Text>
                    <Text type='tertiary'>
                      {t('已使用额度')}: {voucher.used_quota}
                    </Text>
                    <Text type='tertiary'>
                      {t('绑定分组')}:{' '}
                      {activity?.group_name_snapshot || t('未知')}
                    </Text>
                    <Text type='tertiary'>
                      {t('单用户并发上限')}:{' '}
                      {activity?.single_user_concurrency_limit || 0}
                    </Text>
                  </Card>
                );
              })}
            </div>
          )}
        </section>

        <section>
          <Title heading={5}>{t('可领取活动')}</Title>
          {activities.length === 0 ? (
            <Empty description={t('暂无活动福利')} />
          ) : (
            <div className='grid gap-3'>
              {activities.map((activity) => (
                <Card key={activity.id} bodyStyle={{ padding: 16 }}>
                  <div className='flex flex-wrap items-center justify-between gap-3'>
                    <div>
                      <Title heading={6} className='!mb-1'>
                        {activity.name}
                      </Title>
                      <Text type='tertiary'>
                        {activity.group_name_snapshot} ·{' '}
                        {t('共 {{count}} 份', { count: activity.total_count })}
                      </Text>
                    </div>
                    {activity.has_claimed ? (
                      <Tag color='green'>{t('已领取')}</Tag>
                    ) : (
                      <Button
                        theme='solid'
                        type='primary'
                        disabled={!activity.eligible}
                        onClick={() => claim(activity.id)}
                      >
                        {t('领取')}
                      </Button>
                    )}
                  </div>
                </Card>
              ))}
            </div>
          )}
        </section>
      </div>
    </main>
  );
}
