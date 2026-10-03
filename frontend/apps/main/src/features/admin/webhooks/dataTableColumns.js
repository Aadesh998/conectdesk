import { h } from 'vue'
import { RouterLink } from 'vue-router'
import dropdown from './dataTableDropdown.vue'
import { Badge } from '@shared-ui/components/ui/badge'

export const createColumns = (t) => [
  {
    accessorKey: 'name',
    header: function () {
      return h('div', { class: 'text-center' }, t('globals.terms.name'))
    },
    cell: function ({ row }) {
      const kind = row.original.kind || 'custom'
      return h('div', { class: 'text-center' }, [
        h(RouterLink,
          {
            to: { name: 'edit-webhook', params: { id: row.original.id } },
            class: 'text-foreground font-medium hover:underline'
          },
          () => row.getValue('name')
        ),
        // Where it sends, under the name, so the table fits the admin column.
        h('div', { class: 'mt-0.5 text-xs text-muted-foreground whitespace-nowrap' }, t(`admin.webhook.kind.${kind}`))
      ])
    }
  },
  {
    accessorKey: 'url',
    header: function () {
      return h('div', { class: 'text-center' }, t('globals.terms.url'))
    },
    cell: function ({ row }) {
      const url = row.getValue('url')
      return h('div', { class: 'text-center font-mono text-xs mt-1 max-w-[10rem] truncate', title: url }, url)
    }
  },
  {
    accessorKey: 'events',
    enableGlobalFilter: false,
    header: function () {
      return h('div', { class: 'text-center' }, t('globals.terms.event', 2))
    },
    cell: function ({ row }) {
      const events = row.getValue('events')
      return h('div', { class: 'text-center' }, [
        h(
          Badge,
          { variant: 'secondary', class: 'text-xs whitespace-nowrap' },
          () => `${events.length} ${t('globals.terms.event', 2)}`
        )
      ])
    },
    sortingFn: (rowA, rowB) => {
      const a = rowA.original.events?.length || 0
      const b = rowB.original.events?.length || 0
      return a - b
    }
  },
  {
    accessorKey: 'is_active',
    enableGlobalFilter: false,
    header: () => h('div', { class: 'text-center' }, t('globals.terms.status')),
    cell: ({ row }) => {
      const isActive = row.getValue('is_active')
      return h('div', { class: 'text-center' }, [
        h(
          Badge,
          {
            variant: isActive ? 'success' : 'secondary',
            class: 'text-xs whitespace-nowrap'
          },
          () => isActive ? t('globals.terms.active') : t('globals.terms.inactive')
        )
      ])
    }
  },
  {
    id: 'actions',
    enableHiding: false,
    enableSorting: false,
    cell: ({ row }) => {
      const webhook = row.original
      return h(
        'div',
        { class: 'relative' },
        h(dropdown, {
          webhook
        })
      )
    }
  }
]
