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
import { Card } from '@douyinfe/semi-ui';
import { Gauge, HeartPulse, Timer, TimerReset } from 'lucide-react';

import { API } from '../../helpers';
import {
  formatLatency,
  formatSuccessRate,
  formatThroughput,
  getSuccessRateTextColor,
} from '../table/model-pricing/performance/utils';

const PERFORMANCE_WINDOW_HOURS = 24;
const PERFORMANCE_ROW_LIMIT = 6;

const StatMiniCard = ({ icon: Icon, label, value, loading, valueStyle }) => (
  <div className='flex items-center gap-3 px-4 py-3'>
    <span className='flex h-8 w-8 shrink-0 items-center justify-center rounded-lg bg-gray-100 dark:bg-gray-800'>
      <Icon size={14} />
    </span>
    <div className='min-w-0'>
      <div className='text-xs text-gray-500'>{label}</div>
      {loading ? (
        <div className='mt-1 h-5 w-20 animate-pulse rounded bg-gray-200 dark:bg-gray-700' />
      ) : (
        <div
          className='truncate text-base font-semibold tabular-nums'
          style={valueStyle}
        >
          {value}
        </div>
      )}
    </div>
  </div>
);

function toFiniteNumber(value) {
  const parsed = Number(value);
  return Number.isFinite(parsed) ? parsed : 0;
}

function normalizePerformanceRows(models) {
  return (Array.isArray(models) ? models : [])
    .filter((model) => model && model.model_name)
    .map((model) => ({
      model_name: model.model_name,
      avg_ttft_ms: average(
        (Array.isArray(model.series) ? model.series : []).map(
          (point) => point?.avg_ttft_ms,
        ),
      ),
      avg_latency_ms: toFiniteNumber(model.avg_latency_ms),
      avg_tps: toFiniteNumber(model.avg_tps),
      success_rate: toFiniteNumber(model.success_rate),
      request_count: toFiniteNumber(model.request_count),
    }))
    .sort((left, right) => {
      const requestDelta = right.request_count - left.request_count;
      if (requestDelta !== 0) return requestDelta;
      return String(left.model_name).localeCompare(String(right.model_name));
    });
}

function average(values, positiveOnly = true) {
  const numericValues = values.filter(
    (value) => Number.isFinite(value) && (!positiveOnly || value > 0),
  );
  if (numericValues.length === 0) {
    return 0;
  }

  return (
    numericValues.reduce((sum, value) => sum + value, 0) / numericValues.length
  );
}

const PerformanceOverviewPanel = ({ CARD_PROPS, t }) => {
  const [loading, setLoading] = useState(true);
  const [failed, setFailed] = useState(false);
  const [models, setModels] = useState([]);

  useEffect(() => {
    const controller = new AbortController();
    let alive = true;

    const loadPerformance = async () => {
      setLoading(true);
      setFailed(false);
      setModels([]);
      try {
        const response = await API.get('/api/perf-metrics/summary', {
          params: { hours: PERFORMANCE_WINDOW_HOURS },
          signal: controller.signal,
          skipErrorHandler: true,
        });
        if (!alive) return;
        if (!response.data?.success) {
          setFailed(true);
          return;
        }
        const nextModels = response.data?.data?.models;
        setModels(Array.isArray(nextModels) ? nextModels : []);
      } catch (_error) {
        if (!alive || controller.signal.aborted) return;
        setFailed(true);
        setModels([]);
      } finally {
        if (alive) {
          setLoading(false);
        }
      }
    };

    loadPerformance();

    return () => {
      alive = false;
      controller.abort();
    };
  }, []);

  const rows = useMemo(() => normalizePerformanceRows(models), [models]);
  const totalRequests = useMemo(
    () =>
      rows.reduce(
        (sum, row) =>
          sum + (Number.isFinite(row.request_count) ? row.request_count : 0),
        0,
      ),
    [rows],
  );
  const summary = useMemo(
    () => ({
      successRate: average(rows.map((row) => row.success_rate), false),
      avgTtft: Math.round(average(rows.map((row) => row.avg_ttft_ms))),
      avgLatency: Math.round(average(rows.map((row) => row.avg_latency_ms))),
      avgTps: average(rows.map((row) => row.avg_tps)),
    }),
    [rows],
  );

  if (!loading && (failed || rows.length === 0)) {
    return (
      <Card
        {...CARD_PROPS}
        className='!rounded-2xl'
        bodyStyle={{ padding: '1.5rem' }}
      >
        <div className='flex min-h-[11rem] items-center justify-center text-sm text-gray-400'>
          {failed ? t('加载失败') : t('暂无性能数据')}
        </div>
      </Card>
    );
  }

  return (
    <Card
      {...CARD_PROPS}
      className='!rounded-2xl'
      bodyStyle={{ padding: 0 }}
      title={
        <div className='flex w-full flex-col gap-2 sm:flex-row sm:items-end sm:justify-between'>
          <div className='flex min-w-0 items-center gap-2'>
            <span className='flex h-8 w-8 shrink-0 items-center justify-center rounded-lg bg-green-50 text-green-600 dark:bg-green-950/40 dark:text-green-400'>
              <HeartPulse size={16} />
            </span>
            <div className='min-w-0'>
              <div className='text-sm font-semibold'>
                {t('模型广场性能统计')}
              </div>
              <div className='text-xs text-gray-500'>
                {t('平均延迟、首 Token 延迟、TPS 和成功率')}
              </div>
            </div>
          </div>
          <div className='text-xs text-gray-500'>
            {t('调用量')} {totalRequests.toLocaleString()}
          </div>
        </div>
      }
    >
      <div className='grid grid-cols-1 border-b border-gray-200 dark:border-gray-800 sm:grid-cols-2 xl:grid-cols-4'>
        <StatMiniCard
          icon={HeartPulse}
          label={t('成功率')}
          value={formatSuccessRate(summary.successRate)}
          loading={loading}
          valueStyle={{ color: getSuccessRateTextColor(summary.successRate) }}
        />
        <StatMiniCard
          icon={Timer}
          label={t('平均延迟')}
          value={formatLatency(summary.avgLatency)}
          loading={loading}
        />
        <StatMiniCard
          icon={TimerReset}
          label={t('平均首 Token 延迟')}
          value={formatLatency(summary.avgTtft)}
          loading={loading}
        />
        <StatMiniCard
          icon={Gauge}
          label={t('吞吐量')}
          value={formatThroughput(summary.avgTps)}
          loading={loading}
        />
      </div>

      <div className='max-h-96 overflow-auto'>
        <table className='w-full min-w-[900px] border-collapse text-sm'>
          <thead>
            <tr className='bg-gray-50 text-left text-[10px] font-medium uppercase tracking-wider text-gray-500 dark:bg-gray-900/40'>
              <th className='border-b border-gray-200 px-4 py-2 dark:border-gray-800'>
                {t('模型')}
              </th>
              <th className='border-b border-gray-200 px-4 py-2 dark:border-gray-800'>
                {t('调用量')}
              </th>
              <th className='border-b border-gray-200 px-4 py-2 dark:border-gray-800'>
                {t('平均延迟')}
              </th>
              <th className='border-b border-gray-200 px-4 py-2 dark:border-gray-800'>
                {t('首 Token 延迟')}
              </th>
              <th className='border-b border-gray-200 px-4 py-2 dark:border-gray-800'>
                {t('吞吐量')}
              </th>
              <th className='border-b border-gray-200 px-4 py-2 dark:border-gray-800'>
                {t('成功率')}
              </th>
            </tr>
          </thead>
          <tbody>
            {loading
              ? Array.from({ length: PERFORMANCE_ROW_LIMIT }).map(
                  (_, index) => (
                    <tr key={`performance-loading-${index}`}>
                      <td className='border-b border-gray-200 px-4 py-3 dark:border-gray-800'>
                        <div className='h-4 w-40 animate-pulse rounded bg-gray-200 dark:bg-gray-700' />
                      </td>
                      <td className='border-b border-gray-200 px-4 py-3 dark:border-gray-800'>
                        <div className='h-4 w-20 animate-pulse rounded bg-gray-200 dark:bg-gray-700' />
                      </td>
                      <td className='border-b border-gray-200 px-4 py-3 dark:border-gray-800'>
                        <div className='h-4 w-24 animate-pulse rounded bg-gray-200 dark:bg-gray-700' />
                      </td>
                      <td className='border-b border-gray-200 px-4 py-3 dark:border-gray-800'>
                        <div className='h-4 w-24 animate-pulse rounded bg-gray-200 dark:bg-gray-700' />
                      </td>
                      <td className='border-b border-gray-200 px-4 py-3 dark:border-gray-800'>
                        <div className='h-4 w-24 animate-pulse rounded bg-gray-200 dark:bg-gray-700' />
                      </td>
                      <td className='border-b border-gray-200 px-4 py-3 dark:border-gray-800'>
                        <div className='h-4 w-16 animate-pulse rounded bg-gray-200 dark:bg-gray-700' />
                      </td>
                    </tr>
                  ),
                )
              : rows.map((row) => (
                  <tr key={row.model_name}>
                    <td className='border-b border-gray-200 px-4 py-3 dark:border-gray-800'>
                      <div className='min-w-0'>
                        <div className='truncate font-mono text-sm font-medium'>
                          {row.model_name}
                        </div>
                      </div>
                    </td>
                    <td className='border-b border-gray-200 px-4 py-3 font-mono tabular-nums dark:border-gray-800'>
                      {row.request_count > 0
                        ? row.request_count.toLocaleString()
                        : '—'}
                    </td>
                    <td className='border-b border-gray-200 px-4 py-3 font-mono tabular-nums dark:border-gray-800'>
                      {formatLatency(row.avg_latency_ms)}
                    </td>
                    <td className='border-b border-gray-200 px-4 py-3 font-mono tabular-nums dark:border-gray-800'>
                      {formatLatency(row.avg_ttft_ms)}
                    </td>
                    <td className='border-b border-gray-200 px-4 py-3 font-mono tabular-nums dark:border-gray-800'>
                      {formatThroughput(row.avg_tps)}
                    </td>
                    <td
                      className='border-b border-gray-200 px-4 py-3 font-mono tabular-nums dark:border-gray-800'
                      style={{
                        color: getSuccessRateTextColor(row.success_rate),
                      }}
                    >
                      {formatSuccessRate(row.success_rate)}
                    </td>
                  </tr>
                ))}
          </tbody>
        </table>
      </div>
    </Card>
  );
};

export default PerformanceOverviewPanel;
