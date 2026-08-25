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
import SuccessRateSparkline from '../../performance/SuccessRateSparkline';
import {
  formatLatency,
  formatSuccessRate,
  formatThroughput,
  getSuccessRateTextColor,
  normalizePerformanceSeries,
} from '../../performance/utils';

const ModelPerformanceBadge = ({ performance, t }) => {
  if (!performance) return null;

  const { avg_latency_ms, avg_tps, success_rate } = performance;
  const series = normalizePerformanceSeries(performance.series);
  const statusSeries = series.map((point) => ({
    ...point,
    success_rate: Number.isFinite(point.status_rate)
      ? point.status_rate
      : point.success_rate,
  }));
  const statusRate = Number.isFinite(Number(performance.status_rate))
    ? Number(performance.status_rate)
    : success_rate;
  const statusRateText = formatSuccessRate(statusRate);
  const statusRateColor = getSuccessRateTextColor(statusRate);
  const compactThroughput = formatThroughput(avg_tps).replace(' t/s', 't');

  return (
    <div className='classic-pricing-model-performance-badge'>
      <div title={t('平均延迟')} className='min-w-0'>
        <div className='truncate text-[10px] leading-4 text-semi-color-text-2 opacity-60'>
          {t('延迟')}
        </div>
        <div className='whitespace-nowrap font-mono font-normal leading-4 text-semi-color-text-2 opacity-80'>
          {formatLatency(avg_latency_ms)}
        </div>
      </div>
      <div title={t('吞吐量')} className='min-w-0'>
        <div className='truncate text-[10px] leading-4 text-semi-color-text-2 opacity-60'>
          {t('吞吐量')}
        </div>
        <div className='whitespace-nowrap font-mono font-normal leading-4 text-semi-color-text-2 opacity-80'>
          {compactThroughput}
        </div>
      </div>
      <div
        title={`${t('状态')} · ${statusRateText}`}
        className='min-w-0'
        style={{
          '--classic-pricing-performance-status-color': statusRateColor,
        }}
      >
        <div className='classic-pricing-model-performance-status-label truncate text-[10px] leading-4 text-semi-color-text-2 opacity-60'>
          {t('状态')}
        </div>
        <SuccessRateSparkline
          series={statusSeries}
          overall={statusRate}
          maxPoints={3}
          showOverall={false}
          availabilityTone
          signalStyle
          aggregateWindow
          className='classic-pricing-model-performance-signal'
        />
      </div>
    </div>
  );
};

export default React.memo(ModelPerformanceBadge);
