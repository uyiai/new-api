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
import {
  Activity,
  CircleDollarSign,
  Clock3,
  Tags,
  TriangleAlert,
} from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { Card, CardContent } from '@/components/ui/card'
import { StatCard } from '@/features/dashboard/components/ui/stat-card'
import {
  formatInteger,
  formatPercent,
  formatTagQuota,
  formatUseTimeSeconds,
} from '../../lib'
import type { ChannelTagStatsSummary } from '../../types'

interface TagStatsSummaryCardsProps {
  summary?: ChannelTagStatsSummary
  loading?: boolean
  error?: boolean
}

export function TagStatsSummaryCards(props: TagStatsSummaryCardsProps) {
  const { t } = useTranslation()
  const summary = props.summary
  const totalQuota = summary?.total_quota ?? 0
  const requests = summary?.request_count ?? 0
  const untaggedQuota = summary?.untagged_quota ?? 0
  const untaggedRequests = summary?.untagged_request_count ?? 0

  const cards = [
    {
      title: t('Total Quota'),
      value: formatTagQuota(totalQuota),
      description: t('Consumed by all channel tags'),
      icon: CircleDollarSign,
      tone: 'teal' as const,
      details: [
        { label: t('Requests'), value: formatInteger(requests) },
        {
          label: t('Average Latency'),
          value: formatUseTimeSeconds(summary?.average_use_time),
        },
      ],
    },
    {
      title: t('Requests'),
      value: formatInteger(requests),
      description: t('Successful consume logs'),
      icon: Activity,
      tone: 'gray' as const,
      details: [
        { label: t('Channels'), value: formatInteger(summary?.channel_count) },
        {
          label: t('Tag Groups'),
          value: formatInteger(summary?.tag_group_count),
        },
      ],
    },
    {
      title: t('Tokens'),
      value: formatInteger(summary?.tokens),
      description: t('Prompt and completion tokens'),
      icon: Clock3,
      tone: 'teal' as const,
      details: [
        { label: t('Prompt'), value: formatInteger(summary?.prompt_tokens) },
        {
          label: t('Completion'),
          value: formatInteger(summary?.completion_tokens),
        },
      ],
    },
    {
      title: t('Named Tags'),
      value: formatInteger(summary?.tag_count),
      description: t('Excludes untagged usage'),
      icon: Tags,
      tone: 'gray' as const,
      details: [
        {
          label: t('All Groups'),
          value: formatInteger(summary?.tag_group_count),
        },
        {
          label: t('Untagged Share'),
          value: formatPercent(untaggedQuota, totalQuota),
          tone: untaggedQuota > 0 ? ('warning' as const) : ('muted' as const),
        },
      ],
    },
    {
      title: t('Untagged Usage'),
      value: formatTagQuota(untaggedQuota),
      description: t('Displayed as 未设置标签'),
      icon: TriangleAlert,
      tone: untaggedQuota > 0 ? ('rose' as const) : ('gray' as const),
      details: [
        { label: t('Requests'), value: formatInteger(untaggedRequests) },
        {
          label: t('Quota Share'),
          value: formatPercent(untaggedQuota, totalQuota),
          tone: untaggedQuota > 0 ? ('warning' as const) : ('muted' as const),
        },
      ],
    },
  ]

  return (
    <div className='grid gap-3 md:grid-cols-2 xl:grid-cols-5'>
      {cards.map((card) => (
        <Card key={card.title} size='sm' className='min-w-0'>
          <CardContent>
            <StatCard
              {...card}
              loading={props.loading}
              error={props.error}
              sparklineVariant='line'
            />
          </CardContent>
        </Card>
      ))}
    </div>
  )
}
