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
along with this program. If not, see <https://www.gnu.org/licenses/>

For commercial licensing, please contact support@quantumnous.com
*/

import React, { useCallback, useEffect, useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';
import dayjs from 'dayjs';
import {
  Banner,
  Button,
  Card,
  DatePicker,
  Empty,
  Input,
  Space,
  Spin,
  Table,
  TabPane,
  Tabs,
  Tag,
  Typography,
} from '@douyinfe/semi-ui';
import { IconRefresh, IconSearch } from '@douyinfe/semi-icons';
import { VChart } from '@visactor/react-vchart';
import { initVChartSemiTheme } from '@visactor/vchart-semi-theme';
import { Activity, Clock3, CircleDollarSign, Tags } from 'lucide-react';
import { API, renderNumber, renderQuota, showError } from '../../helpers';
import { CHART_CONFIG } from '../../constants/dashboard.constants';
import { DATE_RANGE_PRESETS } from '../../constants/console.constants';

const { Text, Title } = Typography;

const TREND_LIMIT = 12;
const UNTAGGED_CHANNEL_TAG_KEY = '__untagged__';

const GRANULARITY_OPTIONS = [
  { value: 'hour', label: '小时' },
  { value: 'day', label: '天' },
  { value: 'week', label: '周' },
];

const METRIC_OPTIONS = [
  { value: 'quota', label: '额度' },
  { value: 'request_count', label: '请求数' },
  { value: 'tokens', label: 'Token 数' },
];

const defaultDateRange = () => [
  dayjs().subtract(6, 'day').startOf('day').toDate(),
  dayjs().endOf('day').toDate(),
];

const toUnixSeconds = (date) => {
  if (!date) return undefined;
  const timestamp = dayjs(date).unix();
  return Number.isFinite(timestamp) ? timestamp : undefined;
};

const normalizeNumber = (value) => Number(value) || 0;

const formatInteger = (value) =>
  new Intl.NumberFormat(undefined, { maximumFractionDigits: 0 }).format(
    normalizeNumber(value),
  );

const formatPercent = (value, total) => {
  const numericValue = normalizeNumber(value);
  const numericTotal = normalizeNumber(total);
  if (numericTotal <= 0) return '0%';
  return `${((numericValue / numericTotal) * 100).toFixed(1)}%`;
};

const formatUseTimeSeconds = (value) => {
  const seconds = normalizeNumber(value);
  if (seconds <= 0) return '0s';
  if (seconds < 1) return `${seconds.toFixed(2)}s`;
  if (seconds < 10) return `${seconds.toFixed(1)}s`;
  return `${Math.round(seconds)}s`;
};

const formatLastLogTime = (timestamp) => {
  if (!timestamp) return '-';
  return dayjs(timestamp * 1000).format('YYYY-MM-DD HH:mm');
};

const formatMetricValue = (metric, value) => {
  if (metric === 'quota') return renderQuota(normalizeNumber(value), 2);
  return formatInteger(value);
};

const getTagName = (item) => item?.tag_name || '未设置标签';

const getMetricValue = (item, metric) => normalizeNumber(item?.[metric]);

const normalizeSearchText = (value) => String(value || '').trim().toLowerCase();

const tagMatchesKeyword = (item, keyword) =>
  `${item?.tag_name || ''} ${item?.tag_key || ''}`
    .toLowerCase()
    .includes(keyword);

const channelMatchesKeyword = (channel, keyword) =>
  `${channel?.channel_id || ''} ${channel?.channel_name || ''}`
    .toLowerCase()
    .includes(keyword);

const getDisplayChannels = (item) =>
  Array.isArray(item?.filtered_channels)
    ? item.filtered_channels
    : Array.isArray(item?.channels)
      ? item.channels
      : [];

const getChannelStatusText = (status, t) => {
  if (status === 1) return t('已启用');
  if (status === 2) return t('自动禁用');
  return t('已禁用');
};

const getTopItems = (items, metric) =>
  [...items]
    .filter((item) => getMetricValue(item, metric) > 0)
    .sort((a, b) => getMetricValue(b, metric) - getMetricValue(a, metric))
    .slice(0, 12);

const formatBucket = (timestamp, granularity) => {
  if (!timestamp) return '-';
  const date = dayjs(timestamp * 1000);
  if (granularity === 'hour') return date.format('MM-DD HH:mm');
  if (granularity === 'week') return date.format('YYYY-MM-DD');
  return date.format('MM-DD');
};

const buildTooltipFormatter = (metric) => (items) =>
  items.map((item) => ({
    ...item,
    value: formatMetricValue(metric, Number(item.datum?.value ?? item.value)),
  }));

const buildTagQuotaPieSpec = (items, t) => {
  const values = getTopItems(items, 'quota').map((item) => ({
    tag_name: getTagName(item),
    value: normalizeNumber(item.quota),
    request_count: normalizeNumber(item.request_count),
  }));

  return {
    type: 'pie',
    data: [{ id: 'tag-quota-pie', values }],
    categoryField: 'tag_name',
    valueField: 'value',
    outerRadius: 0.82,
    innerRadius: 0.52,
    padAngle: 0.8,
    title: {
      visible: true,
      text: t('标签额度占比'),
      subtext: `${t('总计')}：${renderQuota(
        values.reduce((sum, item) => sum + item.value, 0),
        2,
      )}`,
    },
    legends: { visible: true, orient: 'bottom' },
    label: { visible: values.length > 0, formatter: '{tag_name}' },
    tooltip: {
      mark: {
        title: { value: t('额度') },
        updateContent: buildTooltipFormatter('quota'),
      },
    },
    animation: true,
  };
};

const buildTagMetricBarSpec = (items, metric, t) => {
  const values = getTopItems(items, metric).map((item) => ({
    tag_name: getTagName(item),
    value: getMetricValue(item, metric),
  }));
  const metricLabel = t(
    METRIC_OPTIONS.find((item) => item.value === metric)?.label || '额度',
  );

  return {
    type: 'bar',
    data: [{ id: 'tag-metric-bar', values }],
    xField: 'tag_name',
    yField: 'value',
    padding: { top: 36, right: 12, bottom: 8, left: 8 },
    title: {
      visible: true,
      text: t('标签排行'),
      subtext: metricLabel,
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
        label: { formatter: (value) => formatMetricValue(metric, value) },
      },
    ],
    tooltip: {
      mark: {
        title: { value: metricLabel },
        updateContent: buildTooltipFormatter(metric),
      },
    },
    bar: { style: { cornerRadius: [6, 6, 0, 0] } },
    animation: true,
  };
};

const buildTagTrendSpec = (trend, granularity, metric, t) => {
  const values = [...trend]
    .sort((a, b) => normalizeNumber(a.bucket_start) - normalizeNumber(b.bucket_start))
    .map((point) => ({
      bucket: point.bucket_start,
      bucket_label: formatBucket(point.bucket_start, granularity),
      tag_name: getTagName(point),
      value: getMetricValue(point, metric),
    }));
  const metricLabel = t(
    METRIC_OPTIONS.find((item) => item.value === metric)?.label || '额度',
  );

  return {
    type: 'line',
    data: [{ id: 'tag-trend', values }],
    xField: 'bucket_label',
    yField: 'value',
    seriesField: 'tag_name',
    title: {
      visible: true,
      text: t('标签趋势'),
      subtext: metricLabel,
    },
    point: { visible: values.length <= 80, style: { size: 3 } },
    line: { style: { lineWidth: 2 } },
    axes: [
      {
        orient: 'bottom',
        type: 'band',
        label: { autoRotate: true, autoLimit: true },
      },
      {
        orient: 'left',
        type: 'linear',
        label: { formatter: (value) => formatMetricValue(metric, value) },
      },
    ],
    legends: { visible: true, orient: 'bottom', maxRow: 2 },
    tooltip: {
      dimension: {
        title: { value: (datum) => datum.bucket_label },
        updateContent: buildTooltipFormatter(metric),
      },
    },
    animation: true,
  };
};

const SummaryCard = ({ title, value, description, details, icon: Icon, tone }) => {
  const toneColor =
    tone === 'danger'
      ? 'var(--semi-color-danger)'
      : tone === 'warning'
        ? 'var(--semi-color-warning)'
        : 'var(--semi-color-primary)';

  return (
    <Card className='!rounded-xl min-w-0' bodyStyle={{ padding: 16 }}>
      <div className='flex items-start justify-between gap-3'>
        <div className='min-w-0'>
          <div className='text-sm text-semi-color-text-2'>{title}</div>
          <div className='mt-1 text-2xl font-semibold truncate'>{value}</div>
          <div className='mt-1 text-xs text-semi-color-text-2'>{description}</div>
        </div>
        {Icon ? <Icon size={22} color={toneColor} /> : null}
      </div>
      {details?.length ? (
        <div className='mt-3 flex flex-wrap gap-2 text-xs text-semi-color-text-2'>
          {details.map((detail) => (
            <span key={detail.label} className='rounded bg-semi-color-fill-0 px-2 py-1'>
              {detail.label}: <b className='text-semi-color-text-0'>{detail.value}</b>
            </span>
          ))}
        </div>
      ) : null}
    </Card>
  );
};

const ChartCard = ({ title, description, spec, empty, loading, children, tall }) => {
  const { t } = useTranslation();

  return (
    <Card
      className='!rounded-2xl min-w-0'
      title={
        <div className='flex flex-col gap-2 md:flex-row md:items-center md:justify-between w-full'>
          <div className='min-w-0'>
            <div className='font-semibold'>{title}</div>
            <div className='text-xs text-semi-color-text-2'>{description}</div>
          </div>
          {children ? <div className='shrink-0'>{children}</div> : null}
        </div>
      }
      bodyStyle={{ padding: 12 }}
    >
      <div className={tall ? 'h-96' : 'h-80'}>
        {loading ? (
          <div className='flex h-full items-center justify-center'>
            <Spin tip={t('加载中...')} />
          </div>
        ) : empty ? (
          <div className='flex h-full items-center justify-center rounded-lg border border-dashed border-semi-color-border bg-semi-color-fill-0'>
            <Empty description={t('所选范围暂无统计数据')} />
          </div>
        ) : (
          <VChart
            spec={{ ...spec, background: 'transparent' }}
            option={CHART_CONFIG}
          />
        )}
      </div>
    </Card>
  );
};

const ChannelTagStatsPage = () => {
  const { t } = useTranslation();
  const [dateRange, setDateRange] = useState(defaultDateRange);
  const [granularity, setGranularity] = useState('day');
  const [metric, setMetric] = useState('quota');
  const [keyword, setKeyword] = useState('');
  const [loading, setLoading] = useState(false);
  const [fetching, setFetching] = useState(false);
  const [error, setError] = useState('');
  const [stats, setStats] = useState(null);

  useEffect(() => {
    initVChartSemiTheme({ isWatchingThemeSwitch: true });
  }, []);

  const params = useMemo(() => {
    const [start, end] = Array.isArray(dateRange) ? dateRange : [];
    return {
      start_timestamp: toUnixSeconds(start),
      end_timestamp: toUnixSeconds(end),
      granularity,
      trend_limit: TREND_LIMIT,
    };
  }, [dateRange, granularity]);

  const loadStats = useCallback(
    async (silent = false) => {
      const [start, end] = Array.isArray(dateRange) ? dateRange : [];
      if (start && end && dayjs(start).isAfter(dayjs(end))) {
        showError(t('开始时间不能晚于结束时间'));
        return;
      }

      if (silent) setFetching(true);
      else setLoading(true);
      setError('');
      try {
        const res = await API.get('/api/channel/tag_stats', {
          params,
          skipErrorHandler: true,
        });
        const payload = res.data || {};
        if (!payload.success || !payload.data) {
          throw new Error(payload.message || t('获取渠道标签统计失败'));
        }
        setStats(payload.data);
      } catch (err) {
        const message =
          err?.response?.data?.message || err?.message || t('网络错误');
        setError(message);
        showError(message);
      } finally {
        setLoading(false);
        setFetching(false);
      }
    },
    [dateRange, params, t],
  );

  useEffect(() => {
    loadStats(false);
  }, [loadStats]);

  const items = Array.isArray(stats?.items) ? stats.items : [];
  const trend = Array.isArray(stats?.trend) ? stats.trend : [];
  const summary = stats?.summary || {};
  const totalQuota = normalizeNumber(summary.total_quota);

  const filteredItems = useMemo(() => {
    const text = normalizeSearchText(keyword);
    if (!text) {
      return items.map((item) => ({
        ...item,
        filtered_channels: Array.isArray(item.channels) ? item.channels : [],
      }));
    }
    return items.reduce((acc, item) => {
      const channels = Array.isArray(item.channels) ? item.channels : [];
      if (tagMatchesKeyword(item, text)) {
        acc.push({ ...item, filtered_channels: channels });
        return acc;
      }
      const matchedChannels = channels.filter((channel) =>
        channelMatchesKeyword(channel, text),
      );
      if (matchedChannels.length > 0) {
        acc.push({ ...item, filtered_channels: matchedChannels });
      }
      return acc;
    }, []);
  }, [items, keyword]);

  const pieSpec = useMemo(() => buildTagQuotaPieSpec(filteredItems, t), [filteredItems, t]);
  const barSpec = useMemo(
    () => buildTagMetricBarSpec(filteredItems, metric, t),
    [filteredItems, metric, t],
  );
  const trendSpec = useMemo(
    () => buildTagTrendSpec(trend, stats?.granularity || granularity, metric, t),
    [granularity, metric, stats?.granularity, trend, t],
  );

  const channelColumns = useMemo(
    () => [
      {
        title: t('渠道 ID'),
        dataIndex: 'channel_id',
        width: 110,
        render: (value) => <Text code>#{value}</Text>,
        sorter: (a, b) => normalizeNumber(a.channel_id) - normalizeNumber(b.channel_id),
      },
      {
        title: t('渠道名称'),
        dataIndex: 'channel_name',
        width: 220,
        render: (value) => value || '-',
        sorter: (a, b) =>
          String(a.channel_name || '').localeCompare(String(b.channel_name || '')),
      },
      {
        title: t('状态'),
        dataIndex: 'channel_status',
        width: 110,
        render: (value) => (
          <Tag color={value === 1 ? 'green' : 'grey'}>{getChannelStatusText(value, t)}</Tag>
        ),
      },
      {
        title: t('额度'),
        dataIndex: 'quota',
        width: 140,
        align: 'right',
        render: (value) => renderQuota(normalizeNumber(value), 2),
        sorter: (a, b) => normalizeNumber(a.quota) - normalizeNumber(b.quota),
      },
      {
        title: t('请求数'),
        dataIndex: 'request_count',
        width: 120,
        align: 'right',
        render: formatInteger,
        sorter: (a, b) => normalizeNumber(a.request_count) - normalizeNumber(b.request_count),
      },
      {
        title: t('Token 数'),
        dataIndex: 'tokens',
        width: 140,
        align: 'right',
        render: formatInteger,
        sorter: (a, b) => normalizeNumber(a.tokens) - normalizeNumber(b.tokens),
      },
      {
        title: t('平均耗时'),
        dataIndex: 'average_use_time',
        width: 120,
        align: 'right',
        render: formatUseTimeSeconds,
        sorter: (a, b) =>
          normalizeNumber(a.average_use_time) - normalizeNumber(b.average_use_time),
      },
      {
        title: t('最后日志'),
        dataIndex: 'last_log_at',
        width: 170,
        render: formatLastLogTime,
        sorter: (a, b) => normalizeNumber(a.last_log_at) - normalizeNumber(b.last_log_at),
      },
    ],
    [t],
  );

  const tableColumns = useMemo(
    () => [
      {
        title: t('标签'),
        dataIndex: 'tag_name',
        width: 220,
        render: (_, record) => {
          const isUntagged = record.tag_key === UNTAGGED_CHANNEL_TAG_KEY;
          return (
            <Space wrap>
              <Tag color={isUntagged ? 'grey' : 'blue'}>{getTagName(record)}</Tag>
              {isUntagged ? <Text type='tertiary'>{t('默认标签组')}</Text> : null}
            </Space>
          );
        },
        sorter: (a, b) => getTagName(a).localeCompare(getTagName(b)),
      },
      {
        title: t('额度'),
        dataIndex: 'quota',
        width: 140,
        align: 'right',
        render: (value) => renderQuota(normalizeNumber(value), 2),
        sorter: (a, b) => normalizeNumber(a.quota) - normalizeNumber(b.quota),
      },
      {
        title: t('占比'),
        dataIndex: 'quota_share',
        width: 100,
        align: 'right',
        render: (_, record) => formatPercent(record.quota, totalQuota),
      },
      {
        title: t('请求数'),
        dataIndex: 'request_count',
        width: 120,
        align: 'right',
        render: formatInteger,
        sorter: (a, b) => normalizeNumber(a.request_count) - normalizeNumber(b.request_count),
      },
      {
        title: t('Token 数'),
        dataIndex: 'tokens',
        width: 140,
        align: 'right',
        render: formatInteger,
        sorter: (a, b) => normalizeNumber(a.tokens) - normalizeNumber(b.tokens),
      },
      {
        title: t('Prompt'),
        dataIndex: 'prompt_tokens',
        width: 130,
        align: 'right',
        render: formatInteger,
      },
      {
        title: t('Completion'),
        dataIndex: 'completion_tokens',
        width: 140,
        align: 'right',
        render: formatInteger,
      },
      {
        title: t('平均耗时'),
        dataIndex: 'average_use_time',
        width: 120,
        align: 'right',
        render: formatUseTimeSeconds,
        sorter: (a, b) =>
          normalizeNumber(a.average_use_time) - normalizeNumber(b.average_use_time),
      },
      {
        title: t('渠道数'),
        dataIndex: 'channel_count',
        width: 110,
        align: 'right',
        render: (_, record) => formatInteger(getDisplayChannels(record).length),
        sorter: (a, b) => getDisplayChannels(a).length - getDisplayChannels(b).length,
      },
      {
        title: t('最后日志'),
        dataIndex: 'last_log_at',
        width: 170,
        render: formatLastLogTime,
        sorter: (a, b) => normalizeNumber(a.last_log_at) - normalizeNumber(b.last_log_at),
      },
    ],
    [t, totalQuota],
  );

  const metricTabs = (
    <Tabs type='button' activeKey={metric} onChange={setMetric} size='small'>
      {METRIC_OPTIONS.map((item) => (
        <TabPane key={item.value} itemKey={item.value} tab={t(item.label)} />
      ))}
    </Tabs>
  );

  const expandedRowRender = (record) => {
    const channels = getDisplayChannels(record);
    if (channels.length === 0) {
      return <Empty description={t('没有匹配的渠道')} />;
    }
    return (
      <div className='rounded-lg bg-semi-color-fill-0 p-3'>
        <div className='mb-2 text-xs text-semi-color-text-2'>
          {t('标签下渠道')} · {formatInteger(channels.length)}
        </div>
        <Table
          columns={channelColumns}
          dataSource={channels}
          rowKey={(channel) => channel.channel_id}
          pagination={false}
          size='small'
          scroll={{ x: 1200 }}
          style={{ width: '100%' }}
        />
      </div>
    );
  };

  return (
    <div className='flex w-full max-w-none flex-col gap-4 overflow-x-auto'>
      <div className='flex flex-col gap-2 md:flex-row md:items-center md:justify-between'>
        <div>
          <Title heading={3} style={{ margin: 0 }}>
            {t('渠道标签统计')}
          </Title>
          <Text type='secondary'>
            {t('按当前渠道标签聚合成功消费日志，辅助查看标签用量、请求和趋势。')}
          </Text>
        </div>
        <Button
          icon={<IconRefresh />}
          onClick={() => loadStats(true)}
          loading={fetching}
          disabled={loading}
        >
          {t('刷新')}
        </Button>
      </div>

      <Card className='!rounded-2xl'>
        <div className='flex flex-col gap-3 xl:flex-row xl:items-center xl:justify-between'>
          <div className='flex flex-col gap-2 md:flex-row md:items-center md:flex-wrap'>
            <DatePicker
              size='small'
              type='dateTimeRange'
              placeholder={[t('开始时间'), t('结束时间')]}
              value={dateRange}
              onChange={(value) => setDateRange(value || [])}
              showClear
              presets={DATE_RANGE_PRESETS.map((preset) => ({
                text: t(preset.text),
                start: preset.start(),
                end: preset.end(),
              }))}
              className='w-full md:w-72'
            />
            <Tabs
              type='button'
              activeKey={granularity}
              onChange={setGranularity}
              size='small'
            >
              {GRANULARITY_OPTIONS.map((item) => (
                <TabPane key={item.value} itemKey={item.value} tab={t(item.label)} />
              ))}
            </Tabs>
            <Input
              size='small'
              prefix={<IconSearch />}
              placeholder={t('筛选标签或渠道')}
              value={keyword}
              onChange={setKeyword}
              showClear
              className='w-full md:w-48'
            />
          </div>
          <Space wrap>
            {DATE_RANGE_PRESETS.map((preset) => (
              <Button
                key={preset.text}
                size='small'
                type='tertiary'
                onClick={() => setDateRange([preset.start(), preset.end()])}
              >
                {t(preset.text)}
              </Button>
            ))}
          </Space>
        </div>
      </Card>

      {error ? (
        <Banner
          type='danger'
          closeIcon={null}
          description={error}
        />
      ) : null}

      <div className='grid grid-cols-1 gap-3 md:grid-cols-2 xl:grid-cols-5'>
        <SummaryCard
          title={t('总额度')}
          value={renderQuota(totalQuota, 2)}
          description={t('全部渠道标签消耗')}
          icon={CircleDollarSign}
          details={[
            { label: t('请求数'), value: formatInteger(summary.request_count) },
            { label: t('平均耗时'), value: formatUseTimeSeconds(summary.average_use_time) },
          ]}
        />
        <SummaryCard
          title={t('请求数')}
          value={formatInteger(summary.request_count)}
          description={t('成功消费日志')}
          icon={Activity}
          details={[
            { label: t('渠道数'), value: formatInteger(summary.channel_count) },
            { label: t('标签组'), value: formatInteger(summary.tag_group_count) },
          ]}
        />
        <SummaryCard
          title='Tokens'
          value={formatInteger(summary.tokens)}
          description={t('Prompt 与 Completion 总量')}
          icon={Clock3}
          details={[
            { label: 'Prompt', value: formatInteger(summary.prompt_tokens) },
            { label: 'Completion', value: formatInteger(summary.completion_tokens) },
          ]}
        />
        <SummaryCard
          title={t('命名标签')}
          value={formatInteger(summary.tag_count)}
          description={t('不含未设置标签')}
          icon={Tags}
          details={[
            { label: t('全部标签组'), value: formatInteger(summary.tag_group_count) },
            {
              label: t('未设置占比'),
              value: formatPercent(summary.untagged_quota, totalQuota),
            },
          ]}
        />
        <SummaryCard
          title={t('未设置标签')}
          value={renderQuota(summary.untagged_quota || 0, 2)}
          description={t('空白标签按后端返回的未设置标签展示')}
          icon={Tags}
          tone={summary.untagged_quota > 0 ? 'warning' : 'primary'}
          details={[
            { label: t('请求数'), value: formatInteger(summary.untagged_request_count) },
            {
              label: t('额度占比'),
              value: formatPercent(summary.untagged_quota, totalQuota),
            },
          ]}
        />
      </div>

      <div className='grid grid-cols-1 gap-3 xl:grid-cols-2'>
        <ChartCard
          title={t('标签额度占比')}
          description={t('按额度展示前 12 个渠道标签')}
          spec={pieSpec}
          empty={filteredItems.length === 0}
          loading={loading}
        />
        <ChartCard
          title={t('标签排行')}
          description={t('按所选指标对比渠道标签')}
          spec={barSpec}
          empty={filteredItems.length === 0}
          loading={loading}
        >
          {metricTabs}
        </ChartCard>
        <div className='xl:col-span-2'>
          <ChartCard
            title={t('标签趋势')}
            description={t('趋势线使用当前渠道标签和所选粒度')}
            spec={trendSpec}
            empty={trend.length === 0}
            loading={loading}
            tall
          >
            <Text type='tertiary' size='small'>
              {t('指标')}: {t(METRIC_OPTIONS.find((item) => item.value === metric)?.label || '额度')}
            </Text>
          </ChartCard>
        </div>
      </div>

      <Card className='!rounded-2xl overflow-x-auto' title={t('标签明细')}>
        <Table
          columns={tableColumns}
          dataSource={filteredItems}
          rowKey={(record) => record.tag_key || record.tag_name}
          loading={loading || fetching}
          pagination={{ pageSize: 10 }}
          expandedRowRender={expandedRowRender}
          rowExpandable={(record) => getDisplayChannels(record).length > 0}
          expandRowByClick
          scroll={{ x: 1500 }}
          style={{ width: '100%' }}
          empty={<Empty description={t('所选范围暂无渠道标签统计')} />}
        />
      </Card>
    </div>
  );
};

export default ChannelTagStatsPage;
