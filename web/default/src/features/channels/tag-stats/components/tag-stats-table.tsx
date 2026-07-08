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
import { Fragment, useMemo, useState } from 'react'
import { getRouteApi } from '@tanstack/react-router'
import {
  createColumnHelper,
  flexRender,
  getCoreRowModel,
  getFilteredRowModel,
  getPaginationRowModel,
  getSortedRowModel,
  useReactTable,
  type ColumnDef,
  type Row,
  type SortingState,
  type VisibilityState,
} from '@tanstack/react-table'
import { ArrowUpDown, ChevronDown, ChevronRight, Tags } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { DataTablePage } from '@/components/data-table'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import {
  Table as UiTable,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import { useTableUrlState } from '@/hooks/use-table-url-state'
import { cn } from '@/lib/utils'
import {
  formatInteger,
  formatLastLogTime,
  formatPercent,
  formatTagQuota,
  formatUseTimeSeconds,
  UNTAGGED_CHANNEL_TAG_KEY,
} from '../../lib'
import type { ChannelTagStatsChannelItem, ChannelTagStatsItem } from '../../types'

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

function normalizeSearchText(value: unknown): string {
  return String(value ?? '').trim().toLowerCase()
}

function tagMatches(item: ChannelTagStatsItem, needle: string): boolean {
  return `${item.tag_name} ${item.tag_key}`.toLowerCase().includes(needle)
}

function channelMatches(
  channel: ChannelTagStatsChannelItem,
  needle: string
): boolean {
  return `${channel.channel_id} ${channel.channel_name}`
    .toLowerCase()
    .includes(needle)
}

function filterChannelsForDisplay(
  item: ChannelTagStatsItem,
  filterValue: unknown
): ChannelTagStatsChannelItem[] {
  const needle = normalizeSearchText(filterValue)
  const channels = item.channels ?? []
  if (!needle || tagMatches(item, needle)) return channels
  return channels.filter((channel) => channelMatches(channel, needle))
}

function channelStatusLabel(status: number, t: (key: string) => string): string {
  if (status === 1) return t('Enabled')
  if (status === 2) return t('Auto Disabled')
  return t('Disabled')
}

function ChannelDetailsTable(props: {
  item: ChannelTagStatsItem
  globalFilter: unknown
}) {
  const { t } = useTranslation()
  const channels = filterChannelsForDisplay(props.item, props.globalFilter)

  return (
    <div className='bg-muted/30 rounded-md border p-2'>
      <div className='text-muted-foreground mb-2 text-xs'>
        {t('Channels under this tag')} · {formatInteger(channels.length)}
      </div>
      <UiTable>
        <TableHeader>
          <TableRow>
            <TableHead>{t('Channel ID')}</TableHead>
            <TableHead>{t('Channel')}</TableHead>
            <TableHead>{t('Status')}</TableHead>
            <TableHead className='text-right'>{t('Quota')}</TableHead>
            <TableHead className='text-right'>{t('Requests')}</TableHead>
            <TableHead className='text-right'>{t('Tokens')}</TableHead>
            <TableHead className='text-right'>{t('Avg Latency')}</TableHead>
            <TableHead>{t('Last Log')}</TableHead>
          </TableRow>
        </TableHeader>
        <TableBody>
          {channels.length === 0 ? (
            <TableRow>
              <TableCell colSpan={8} className='text-muted-foreground text-center'>
                {t('No channels matched the current filter.')}
              </TableCell>
            </TableRow>
          ) : (
            channels.map((channel) => (
              <TableRow key={channel.channel_id}>
                <TableCell className='font-mono tabular-nums'>
                  #{channel.channel_id}
                </TableCell>
                <TableCell className='max-w-[260px] truncate'>
                  {channel.channel_name || '-'}
                </TableCell>
                <TableCell>
                  <Badge
                    variant={channel.channel_status === 1 ? 'outline' : 'secondary'}
                  >
                    {channelStatusLabel(channel.channel_status, t)}
                  </Badge>
                </TableCell>
                <TableCell className='text-right font-mono tabular-nums'>
                  {formatTagQuota(channel.quota)}
                </TableCell>
                <TableCell className='text-right font-mono tabular-nums'>
                  {formatInteger(channel.request_count)}
                </TableCell>
                <TableCell className='text-right font-mono tabular-nums'>
                  {formatInteger(channel.tokens)}
                </TableCell>
                <TableCell className='text-right tabular-nums'>
                  {formatUseTimeSeconds(channel.average_use_time)}
                </TableCell>
                <TableCell className='text-muted-foreground tabular-nums'>
                  {formatLastLogTime(channel.last_log_at)}
                </TableCell>
              </TableRow>
            ))
          )}
        </TableBody>
      </UiTable>
    </div>
  )
}

export function TagStatsTable(props: TagStatsTableProps) {
  const { t } = useTranslation()
  const [expandedTagKeys, setExpandedTagKeys] = useState<Record<string, boolean>>(
    {}
  )
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
      columnHelper.display({
        id: 'expand',
        header: '',
        cell: ({ row }) => {
          const tagKey = row.original.tag_key || row.original.tag_name
          const expanded = !!expandedTagKeys[tagKey]
          const channelCount = row.original.channels?.length ?? 0
          return (
            <Button
              type='button'
              variant='ghost'
              size='icon'
              className='size-7'
              aria-label={expanded ? t('Collapse channels') : t('Expand channels')}
              disabled={channelCount === 0}
              onClick={() =>
                setExpandedTagKeys((previous) => ({
                  ...previous,
                  [tagKey]: !previous[tagKey],
                }))
              }
            >
              {expanded ? <ChevronDown /> : <ChevronRight />}
            </Button>
          )
        },
      }),
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
    [expandedTagKeys, props.totalQuota, t]
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
    globalFilterFn: (row, _columnId, filterValue) => {
      const needle = normalizeSearchText(filterValue)
      if (!needle) return true
      return (
        tagMatches(row.original, needle) ||
        (row.original.channels ?? []).some((channel) =>
          channelMatches(channel, needle)
        )
      )
    },
    getCoreRowModel: getCoreRowModel(),
    getFilteredRowModel: getFilteredRowModel(),
    getSortedRowModel: getSortedRowModel(),
    getPaginationRowModel: getPaginationRowModel(),
  })

  const renderRow = (row: Row<ChannelTagStatsItem>) => {
    const tagKey = row.original.tag_key || row.original.tag_name
    const expanded = !!expandedTagKeys[tagKey]
    return (
      <Fragment key={row.id}>
        <TableRow data-state={row.getIsSelected() && 'selected'}>
          {row.getVisibleCells().map((cell) => (
            <TableCell
              key={cell.id}
              className={cn(cell.column.id === 'expand' && 'w-8 pr-0')}
            >
              {flexRender(cell.column.columnDef.cell, cell.getContext())}
            </TableCell>
          ))}
        </TableRow>
        {expanded ? (
          <TableRow className='hover:bg-transparent'>
            <TableCell colSpan={row.getVisibleCells().length} className='bg-muted/20'>
              <ChannelDetailsTable item={row.original} globalFilter={globalFilter} />
            </TableCell>
          </TableRow>
        ) : null}
      </Fragment>
    )
  }

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
      renderRow={renderRow}
      toolbarProps={{
        searchPlaceholder: t('Search tags or channels...'),
      }}
      skeletonKeyPrefix='channel-tag-stats-skeleton'
      paginationInFooter={false}
      tableClassName='bg-card'
    />
  )
}
