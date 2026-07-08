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
import { useEffect, useMemo, useRef, useState } from 'react'
import { VChart } from '@visactor/react-vchart'
import { useTranslation } from 'react-i18next'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card'
import { Skeleton } from '@/components/ui/skeleton'
import { Tabs, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { useTheme } from '@/context/theme-provider'
import { VCHART_OPTION } from '@/lib/vchart'
import {
  buildTagMetricBarSpec,
  buildTagQuotaPieSpec,
  buildTagTrendSpec,
  CHANNEL_TAG_STATS_METRICS,
  type ChannelTagStatsMetric,
  metricLabel,
} from '../../lib'
import type {
  ChannelTagStatsGranularity,
  ChannelTagStatsItem,
  ChannelTagStatsTrendPoint,
} from '../../types'

let themeManagerPromise: Promise<
  (typeof import('@visactor/vchart'))['ThemeManager']
> | null = null

interface TagStatsChartsProps {
  items: ChannelTagStatsItem[]
  trend: ChannelTagStatsTrendPoint[]
  granularity: ChannelTagStatsGranularity
  loading?: boolean
}

interface ChartCardProps {
  title: string
  description: string
  chartKey: string
  spec: Record<string, unknown>
  empty: boolean
  loading?: boolean
  children?: React.ReactNode
  heightClassName?: string
}

function ChartCard(props: ChartCardProps) {
  const { t } = useTranslation()
  const { resolvedTheme } = useTheme()
  const [themeReady, setThemeReady] = useState(false)
  const themeManagerRef = useRef<
    (typeof import('@visactor/vchart'))['ThemeManager'] | null
  >(null)

  useEffect(() => {
    const updateTheme = async () => {
      setThemeReady(false)
      if (!themeManagerPromise) {
        themeManagerPromise = import('@visactor/vchart').then(
          (module) => module.ThemeManager
        )
      }
      const ThemeManager = await themeManagerPromise
      themeManagerRef.current = ThemeManager
      ThemeManager.setCurrentTheme(resolvedTheme === 'dark' ? 'dark' : 'light')
      setThemeReady(true)
    }

    updateTheme()
  }, [resolvedTheme])

  return (
    <Card className='min-w-0'>
      <CardHeader className='gap-2 sm:grid-cols-[1fr_auto]'>
        <div className='min-w-0'>
          <CardTitle>{props.title}</CardTitle>
          <CardDescription>{props.description}</CardDescription>
        </div>
        {props.children ? <div>{props.children}</div> : null}
      </CardHeader>
      <CardContent>
        <div className={props.heightClassName ?? 'h-80'}>
          {props.loading ? (
            <Skeleton className='size-full rounded-lg' />
          ) : props.empty ? (
            <div className='bg-muted/30 text-muted-foreground flex size-full items-center justify-center rounded-lg border border-dashed text-sm'>
              {t('No statistics for the selected range')}
            </div>
          ) : themeReady ? (
            <VChart
              key={`${props.chartKey}-${resolvedTheme}`}
              spec={{
                ...props.spec,
                theme: resolvedTheme === 'dark' ? 'dark' : 'light',
                background: 'transparent',
              }}
              option={VCHART_OPTION}
            />
          ) : null}
        </div>
      </CardContent>
    </Card>
  )
}

export function TagStatsCharts(props: TagStatsChartsProps) {
  const { t } = useTranslation()
  const [metric, setMetric] = useState<ChannelTagStatsMetric>('quota')
  const hasItems = props.items.length > 0
  const hasTrend = props.trend.length > 0

  const pieSpec = useMemo(
    () => buildTagQuotaPieSpec(props.items, t),
    [props.items, t]
  )
  const barSpec = useMemo(
    () => buildTagMetricBarSpec(props.items, metric, t),
    [metric, props.items, t]
  )
  const trendSpec = useMemo(
    () => buildTagTrendSpec(props.trend, props.granularity, metric, t),
    [metric, props.granularity, props.trend, t]
  )

  const metricTabs = (
    <Tabs
      value={metric}
      onValueChange={(value) => setMetric(value as ChannelTagStatsMetric)}
    >
      <TabsList>
        {CHANNEL_TAG_STATS_METRICS.map((item) => (
          <TabsTrigger key={item.value} value={item.value}>
            {t(item.label)}
          </TabsTrigger>
        ))}
      </TabsList>
    </Tabs>
  )

  return (
    <div className='grid gap-3 xl:grid-cols-2'>
      <ChartCard
        title={t('Quota Share by Tag')}
        description={t('Top channel tags by consumed quota')}
        chartKey={`pie-${props.items.length}`}
        spec={pieSpec}
        empty={!hasItems}
        loading={props.loading}
      />

      <ChartCard
        title={t('Tag Ranking')}
        description={t('Compare the strongest channel tags by metric')}
        chartKey={`bar-${metric}-${props.items.length}`}
        spec={barSpec}
        empty={!hasItems}
        loading={props.loading}
      >
        {metricTabs}
      </ChartCard>

      <div className='xl:col-span-2'>
        <ChartCard
          title={t('Usage Trend by Tag')}
          description={t('Trend lines use current channel tags and selected granularity')}
          chartKey={`trend-${metric}-${props.granularity}-${props.trend.length}`}
          spec={trendSpec}
          empty={!hasTrend}
          loading={props.loading}
          heightClassName='h-96'
        >
          <div className='text-muted-foreground text-xs'>
            {t('Metric')}: {metricLabel(metric, t)}
          </div>
        </ChartCard>
      </div>
    </div>
  )
}
