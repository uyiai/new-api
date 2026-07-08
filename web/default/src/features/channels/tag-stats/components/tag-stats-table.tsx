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
import { useMemo, useState } from 'react'
import { getRouteApi } from '@tanstack/react-router'
import {
  createColumnHelper,
  getCoreRowModel,
  getFilteredRowModel,
  getPaginationRowModel,
  getSortedRowModel,
  useReactTable,
  type ColumnDef,
  type SortingState,
  type VisibilityState,
} from '@tanstack/react-table'
import { ArrowUpDown, Tags } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { DataTablePage } from '@/components/data-table'
import { useTableUrlState } from '@/hooks/use-table-url-state'
import {
  formatInteger,
  formatLastLogTime,
  formatPercent,
  formatTagQuota,
  formatUseTimeSeconds,
  UNTAGGED_CHANNEL_TAG_KEY,
} from '../../lib'
import type { ChannelTagStatsItem } from '../../types'

const route = getRouteApi('/_authenticated/channels/tag-stats')
const columnHelper = createColumnHelper<ChannelTagStatsItem>()

interface TagStatsTableProps {
  items: ChannelTagStatsItem[]
  totalQuota: number
  loading?: boolean
  fetching?: boolean
}

function SortableHeader(props: {
  label: string
  onClick: () => void
  numeric?: boolean
}) {
  return (
    <Button
      type='button'
      variant='ghost'
      size='sm'
      className={props.numeric ? 'ml-auto' : undefined}
      onClick={props.onClick}
    >
      {props.label}
      <ArrowUpDown data-icon='inline-end' />
    </Button>
  )
}

export function TagStatsTable(props: TagStatsTableProps) {
  const { t } = useTranslation()
  const [sorting, setSorting] = useState<SortingState>([
    { id: 'quota', desc: true },
  ])
  const [columnVisibility, setColumnVisibility] = useState<VisibilityState>({
    prompt_tokens: false,
    completion_tokens: false,
  })

  const {
    globalFilter,
    onGlobalFilterChange,
    pagination,
    onPaginationChange,
  } = useTableUrlState({
    search: route.useSearch(),
    navigate: route.useNavigate(),
    pagination: { defaultPage: 1, defaultPageSize: 10 },
    globalFilter: { enabled: true, key: 'filter' },
  })

  const columns = useMemo<ColumnDef<ChannelTagStatsItem, unknown>[]>(
    () => [
      columnHelper.accessor('tag_name', {
        id: 'tag_name',
        header: ({ column }) => (
          <SortableHeader
            label={t('Tag')}
            onClick={() => column.toggleSorting(column.getIsSorted() === 'asc')}
          />
        ),
        cell: ({ row }) => {
          const item = row.original
          const isUntagged = item.tag_key === UNTAGGED_CHANNEL_TAG_KEY
          return (
            <div className='flex min-w-0 items-center gap-2'>
              <Badge variant={isUntagged ? 'secondary' : 'outline'}>
                {item.tag_name}
              </Badge>
              {isUntagged ? (
                <span className='text-muted-foreground text-xs'>
                  {t('Default tag group')}
                </span>
              ) : null}
            </div>
          )
        },
      }),
      columnHelper.accessor('quota', {
        header: ({ column }) => (
          <SortableHeader
            label={t('Quota')}
            numeric
            onClick={() => column.toggleSorting(column.getIsSorted() === 'asc')}
          />
        ),
        cell: ({ row }) => (
          <div className='text-right font-mono tabular-nums'>
            {formatTagQuota(row.original.quota)}
          </div>
        ),
      }),
      columnHelper.display({
        id: 'quota_share',
        header: () => <div className='text-right'>{t('Share')}</div>,
        cell: ({ row }) => (
          <div className='text-muted-foreground text-right text-xs tabular-nums'>
            {formatPercent(row.original.quota, props.totalQuota)}
          </div>
        ),
      }),
      columnHelper.accessor('request_count', {
        header: ({ column }) => (
          <SortableHeader
            label={t('Requests')}
            numeric
            onClick={() => column.toggleSorting(column.getIsSorted() === 'asc')}
          />
        ),
        cell: ({ row }) => (
          <div className='text-right font-mono tabular-nums'>
            {formatInteger(row.original.request_count)}
          </div>
        ),
      }),
      columnHelper.accessor('tokens', {
        header: ({ column }) => (
          <SortableHeader
            label={t('Tokens')}
            numeric
            onClick={() => column.toggleSorting(column.getIsSorted() === 'asc')}
          />
        ),
        cell: ({ row }) => (
          <div className='text-right font-mono tabular-nums'>
            {formatInteger(row.original.tokens)}
          </div>
        ),
      }),
      columnHelper.accessor('prompt_tokens', {
        header: t('Prompt'),
        cell: ({ row }) => formatInteger(row.original.prompt_tokens),
      }),
      columnHelper.accessor('completion_tokens', {
        header: t('Completion'),
        cell: ({ row }) => formatInteger(row.original.completion_tokens),
      }),
      columnHelper.accessor('average_use_time', {
        header: ({ column }) => (
          <SortableHeader
            label={t('Avg Latency')}
            numeric
            onClick={() => column.toggleSorting(column.getIsSorted() === 'asc')}
          />
        ),
        cell: ({ row }) => (
          <div className='text-right tabular-nums'>
            {formatUseTimeSeconds(row.original.average_use_time)}
          </div>
        ),
      }),
      columnHelper.accessor('channel_count', {
        header: ({ column }) => (
          <SortableHeader
            label={t('Channels')}
            numeric
            onClick={() => column.toggleSorting(column.getIsSorted() === 'asc')}
          />
        ),
        cell: ({ row }) => (
          <div className='text-right tabular-nums'>
            {formatInteger(row.original.channel_count)}
          </div>
        ),
      }),
      columnHelper.accessor('last_log_at', {
        header: ({ column }) => (
          <SortableHeader
            label={t('Last Log')}
            onClick={() => column.toggleSorting(column.getIsSorted() === 'asc')}
          />
        ),
        cell: ({ row }) => (
          <span className='text-muted-foreground tabular-nums'>
            {formatLastLogTime(row.original.last_log_at)}
          </span>
        ),
      }),
    ],
    [props.totalQuota, t]
  )

  const table = useReactTable({
    data: props.items,
    columns,
    state: {
      sorting,
      columnVisibility,
      pagination,
      globalFilter,
    },
    onSortingChange: setSorting,
    onColumnVisibilityChange: setColumnVisibility,
    onPaginationChange,
    onGlobalFilterChange,
    getCoreRowModel: getCoreRowModel(),
    getFilteredRowModel: getFilteredRowModel(),
    getSortedRowModel: getSortedRowModel(),
    getPaginationRowModel: getPaginationRowModel(),
  })

  return (
    <DataTablePage
      table={table}
      columns={columns}
      isLoading={props.loading}
      isFetching={props.fetching}
      emptyTitle={t('No Channel Tag Statistics')}
      emptyDescription={t(
        'No usage logs matched the selected date range. Try widening the filter.'
      )}
      emptyIcon={<Tags />}
      toolbarProps={{
        searchPlaceholder: t('Filter by tag...'),
      }}
      skeletonKeyPrefix='channel-tag-stats-skeleton'
      paginationInFooter={false}
      tableClassName='bg-card'
    />
  )
}
