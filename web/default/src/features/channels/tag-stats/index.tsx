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
import { useMemo } from 'react'
import { useQuery } from '@tanstack/react-query'
import { getRouteApi } from '@tanstack/react-router'
import { RefreshCcw } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { SectionPageLayout } from '@/components/layout'
import { Button } from '@/components/ui/button'
import { Card, CardContent } from '@/components/ui/card'
import { Tabs, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { CompactDateTimeRangePicker } from '@/features/usage-logs/components/compact-date-time-range-picker'
import { getRollingDateRange } from '@/lib/time'
import { TIME_RANGE_PRESETS } from '@/features/dashboard/constants'
import { getChannelTagStats } from '../api'
import {
  CHANNEL_TAG_STATS_TREND_LIMIT,
  getChannelTagStatsQueryKey,
} from '../lib'
import type { ChannelTagStatsGranularity } from '../types'
import { TagStatsCharts } from './components/tag-stats-charts'
import { TagStatsSummaryCards } from './components/tag-stats-summary-cards'
import { TagStatsTable } from './components/tag-stats-table'

const route = getRouteApi('/_authenticated/channels/tag-stats')

const CHANNEL_TAG_STATS_GRANULARITY_OPTIONS: Array<{
  value: ChannelTagStatsGranularity
  label: string
}> = [
  { value: 'hour', label: 'Hourly' },
  { value: 'day', label: 'Daily' },
  { value: 'week', label: 'Weekly' },
]

function toDate(value?: number): Date | undefined {
  if (typeof value !== 'number' || !Number.isFinite(value) || value <= 0) {
    return undefined
  }
  const date = new Date(value)
  return Number.isNaN(date.getTime()) ? undefined : date
}

function toUnixSeconds(date?: Date): number | undefined {
  if (!date) return undefined
  return Math.floor(date.getTime() / 1000)
}

export function ChannelTagStats() {
  const { t } = useTranslation()
  const search = route.useSearch()
  const navigate = route.useNavigate()

  const defaultRange = useMemo(() => getRollingDateRange(7), [])
  const startDate = toDate(search.startTime) ?? defaultRange.start
  const endDate = toDate(search.endTime) ?? defaultRange.end
  const granularity = (search.granularity ?? 'day') as ChannelTagStatsGranularity

  const params = useMemo(
    () => ({
      start_timestamp: toUnixSeconds(startDate),
      end_timestamp: toUnixSeconds(endDate),
      granularity,
      trend_limit: CHANNEL_TAG_STATS_TREND_LIMIT,
    }),
    [endDate, granularity, startDate]
  )

  const statsQuery = useQuery({
    queryKey: getChannelTagStatsQueryKey(params),
    queryFn: async () => {
      const response = await getChannelTagStats(params)
      if (!response.success || !response.data) {
        throw new Error(response.message || t('Failed to load channel tag statistics'))
      }
      return response.data
    },
    placeholderData: (previousData) => previousData,
  })

  const data = statsQuery.data
  const items = data?.items ?? []
  const trend = data?.trend ?? []
  const summary = data?.summary

  const updateRange = (range: { start?: Date; end?: Date }) => {
    navigate({
      search: (prev) => ({
        ...prev,
        startTime: range.start?.getTime(),
        endTime: range.end?.getTime(),
        page: undefined,
      }),
    })
  }

  const applyPreset = (days: number) => {
    const range = getRollingDateRange(days)
    updateRange(range)
  }

  const updateGranularity = (value: string) => {
    navigate({
      search: (prev) => ({
        ...prev,
        granularity: value as ChannelTagStatsGranularity,
        page: undefined,
      }),
    })
  }

  return (
    <SectionPageLayout>
      <SectionPageLayout.Title>{t('Channel Tag Stats')}</SectionPageLayout.Title>
      <SectionPageLayout.Actions>
        <Button
          type='button'
          variant='outline'
          onClick={() => statsQuery.refetch()}
          disabled={statsQuery.isFetching}
        >
          <RefreshCcw data-icon='inline-start' />
          {t('Refresh')}
        </Button>
      </SectionPageLayout.Actions>
      <SectionPageLayout.Content>
        <div className='flex flex-col gap-3'>
          <Card>
            <CardContent className='flex flex-col gap-3 md:flex-row md:items-center md:justify-between'>
              <div className='flex min-w-0 flex-1 flex-col gap-2 lg:flex-row lg:items-center'>
                <CompactDateTimeRangePicker
                  start={startDate}
                  end={endDate}
                  onChange={updateRange}
                  className='lg:w-[360px]'
                />
                <div className='flex flex-wrap gap-1.5'>
                  {TIME_RANGE_PRESETS.map((preset) => (
                    <Button
                      key={preset.days}
                      type='button'
                      variant='secondary'
                      size='sm'
                      onClick={() => applyPreset(preset.days)}
                    >
                      {t(preset.label)}
                    </Button>
                  ))}
                </div>
              </div>

              <Tabs value={granularity} onValueChange={updateGranularity}>
                <TabsList>
                  {CHANNEL_TAG_STATS_GRANULARITY_OPTIONS.map((option) => (
                    <TabsTrigger key={option.value} value={option.value}>
                      {t(option.label)}
                    </TabsTrigger>
                  ))}
                </TabsList>
              </Tabs>
            </CardContent>
          </Card>

          {statsQuery.isError ? (
            <Card className='border-destructive/40'>
              <CardContent className='text-destructive text-sm'>
                {statsQuery.error instanceof Error
                  ? statsQuery.error.message
                  : t('Failed to load channel tag statistics')}
              </CardContent>
            </Card>
          ) : null}

          <TagStatsSummaryCards
            summary={summary}
            loading={statsQuery.isLoading}
            error={statsQuery.isError}
          />

          <TagStatsCharts
            items={items}
            trend={trend}
            granularity={data?.granularity ?? granularity}
            loading={statsQuery.isLoading}
          />

          <TagStatsTable
            items={items}
            totalQuota={summary?.total_quota ?? 0}
            loading={statsQuery.isLoading}
            fetching={statsQuery.isFetching}
          />
        </div>
      </SectionPageLayout.Content>
    </SectionPageLayout>
  )
}
