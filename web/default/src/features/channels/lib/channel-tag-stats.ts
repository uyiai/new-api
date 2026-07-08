/*
Copyright (C) 2023-2026 QuantumNous

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
import dayjs from '@/lib/dayjs'
import { formatQuotaWithCurrency } from '@/lib/currency'
import { formatChartTime } from '@/lib/time'
import type {
  ChannelTagStatsGranularity,
  ChannelTagStatsItem,
  ChannelTagStatsParams,
  ChannelTagStatsTrendPoint,
} from '../types'

export const CHANNEL_TAG_STATS_QUERY_KEY = 'channel-tag-stats'
export const CHANNEL_TAG_STATS_TREND_LIMIT = 12
export const UNTAGGED_CHANNEL_TAG_KEY = '__untagged__'

export type ChannelTagStatsMetric = 'quota' | 'request_count' | 'tokens'

type TFunction = (key: string) => string

type VChartTooltipItem = {
  key: string
  value: string | number
  datum?: Record<string, unknown>
}

const INTEGER_FORMATTER = new Intl.NumberFormat(undefined, {
  maximumFractionDigits: 0,
})

export const CHANNEL_TAG_STATS_METRICS: Array<{
  value: ChannelTagStatsMetric
  label: string
}> = [
  { value: 'quota', label: 'Quota' },
  { value: 'request_count', label: 'Requests' },
  { value: 'tokens', label: 'Tokens' },
]

export function getChannelTagStatsQueryKey(params: ChannelTagStatsParams) {
  return [CHANNEL_TAG_STATS_QUERY_KEY, params] as const
}

export function formatInteger(value: number | null | undefined): string {
  return INTEGER_FORMATTER.format(Number(value) || 0)
}

export function formatTagQuota(value: number | null | undefined): string {
  return formatQuotaWithCurrency(Number(value) || 0, {
    digitsLarge: 2,
    digitsSmall: 4,
    abbreviate: true,
  })
}

export function formatPercent(value: number, total: number): string {
  if (!Number.isFinite(value) || !Number.isFinite(total) || total <= 0) {
    return '0%'
  }
  return `${((value / total) * 100).toFixed(1)}%`
}

export function formatUseTimeSeconds(value: number | null | undefined): string {
  const seconds = Number(value) || 0
  if (seconds <= 0) return '0s'
  if (seconds < 1) return `${seconds.toFixed(2)}s`
  if (seconds < 10) return `${seconds.toFixed(1)}s`
  return `${Math.round(seconds)}s`
}

export function formatLastLogTime(timestamp: number): string {
  if (!timestamp) return '-'
  return dayjs(timestamp * 1000).format('YYYY-MM-DD HH:mm')
}

export function metricLabel(metric: ChannelTagStatsMetric, t: TFunction): string {
  const option = CHANNEL_TAG_STATS_METRICS.find((item) => item.value === metric)
  return t(option?.label ?? 'Quota')
}

export function formatMetricValue(
  metric: ChannelTagStatsMetric,
  value: number
): string {
  if (metric === 'quota') return formatTagQuota(value)
  return formatInteger(value)
}

function metricValue(
  item: ChannelTagStatsItem | ChannelTagStatsTrendPoint,
  metric: ChannelTagStatsMetric
): number {
  return Number(item[metric]) || 0
}

function topItems(items: ChannelTagStatsItem[], metric: ChannelTagStatsMetric) {
  return [...items]
    .filter((item) => metricValue(item, metric) > 0)
    .sort((a, b) => metricValue(b, metric) - metricValue(a, metric))
    .slice(0, 12)
}

function tooltipFormatter(metric: ChannelTagStatsMetric, t: TFunction) {
  return (items: VChartTooltipItem[]) =>
    items.map((item) => ({
      ...item,
      key: item.key ? t(item.key) : metricLabel(metric, t),
      value: formatMetricValue(
        metric,
        Number(item.datum?.value ?? item.value) || 0
      ),
    }))
}

export function buildTagQuotaPieSpec(
  items: ChannelTagStatsItem[],
  t: TFunction
) {
  const values = topItems(items, 'quota').map((item) => ({
    tag_name: item.tag_name,
    value: item.quota,
    quota_label: formatTagQuota(item.quota),
    request_count: item.request_count,
  }))

  return {
    type: 'pie',
    data: [{ id: 'tag-quota-pie', values }],
    categoryField: 'tag_name',
    valueField: 'value',
    outerRadius: 0.82,
    innerRadius: 0.52,
    padAngle: 0.8,
    legends: {
      visible: true,
      orient: 'bottom',
      item: { visible: true },
    },
    tooltip: {
      mark: {
        title: { value: t('Quota') },
        updateContent: tooltipFormatter('quota', t),
      },
    },
    label: {
      visible: values.length > 0,
      formatter: '{tag_name}',
    },
    animation: true,
  }
}

export function buildTagMetricBarSpec(
  items: ChannelTagStatsItem[],
  metric: ChannelTagStatsMetric,
  t: TFunction
) {
  const values = topItems(items, metric).map((item) => ({
    tag_name: item.tag_name,
    value: metricValue(item, metric),
  }))

  return {
    type: 'bar',
    data: [{ id: 'tag-metric-bar', values }],
    xField: 'tag_name',
    yField: 'value',
    padding: { top: 16, right: 12, bottom: 8, left: 8 },
    axes: [
      {
        orient: 'bottom',
        type: 'band',
        label: { autoRotate: true, autoLimit: true },
      },
      {
        orient: 'left',
        type: 'linear',
        label: {
          formatter: (value: number) => formatMetricValue(metric, value),
        },
      },
    ],
    tooltip: {
      mark: {
        title: { value: metricLabel(metric, t) },
        updateContent: tooltipFormatter(metric, t),
      },
    },
    bar: {
      style: {
        cornerRadius: [6, 6, 0, 0],
      },
    },
    animation: true,
  }
}

export function buildTagTrendSpec(
  trend: ChannelTagStatsTrendPoint[],
  granularity: ChannelTagStatsGranularity,
  metric: ChannelTagStatsMetric,
  t: TFunction
) {
  const values = [...trend]
    .sort((a, b) => a.bucket_start - b.bucket_start)
    .map((point) => ({
      bucket: point.bucket_start,
      bucket_label: formatChartTime(point.bucket_start, granularity),
      tag_name: point.tag_name,
      value: metricValue(point, metric),
    }))

  return {
    type: 'line',
    data: [{ id: 'tag-trend', values }],
    xField: 'bucket_label',
    yField: 'value',
    seriesField: 'tag_name',
    point: {
      visible: values.length <= 80,
      style: { size: 3 },
    },
    line: {
      style: {
        lineWidth: 2,
      },
    },
    axes: [
      {
        orient: 'bottom',
        type: 'band',
        label: { autoRotate: true, autoLimit: true },
      },
      {
        orient: 'left',
        type: 'linear',
        label: {
          formatter: (value: number) => formatMetricValue(metric, value),
        },
      },
    ],
    legends: {
      visible: true,
      orient: 'bottom',
      maxRow: 2,
    },
    tooltip: {
      dimension: {
        title: { value: (datum: Record<string, unknown>) => datum.bucket_label },
        updateContent: tooltipFormatter(metric, t),
      },
    },
    animation: true,
  }
}
