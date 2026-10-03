<template>
  <AdminSplitLayout>
    <template #content>
      <div class="space-y-6" :class="{ 'opacity-50 transition-opacity duration-300': !data }">
        <div class="flex flex-wrap items-center justify-between gap-3">
          <div class="space-y-1">
            <p class="text-sm text-muted-foreground">{{ $t('admin.usage.description') }}</p>
            <p v-if="data?.plan" class="text-sm">
              {{ $t('admin.usage.plan') }}: <Badge variant="secondary" class="capitalize">{{ data.plan }}</Badge>
            </p>
          </div>
          <a v-if="data?.upgrade_url && data?.plan" :href="data.upgrade_url" target="_blank" rel="noopener">
            <Button size="sm">{{ $t('admin.usage.upgrade') }}</Button>
          </a>
        </div>

        <div v-if="data" class="grid gap-4 sm:grid-cols-2">
          <div v-for="m in meters" :key="m.key" class="rounded-lg border p-4 space-y-3">
            <div class="flex items-center justify-between gap-2">
              <span class="text-sm font-medium">{{ m.label }}</span>
              <Badge v-if="m.ratio >= 1" variant="destructive">{{ $t('admin.usage.atLimit') }}</Badge>
              <Badge v-else-if="m.ratio >= 0.8" variant="outline" class="text-warning-600">
                {{ $t('admin.usage.nearLimit') }}
              </Badge>
            </div>
            <p class="text-2xl font-semibold tabular-nums">
              {{ m.limit ? $t('admin.usage.of', { used: m.used, limit: m.limitText }) : m.used }}
            </p>
            <div
              v-if="m.limit"
              class="h-1.5 rounded-full bg-muted overflow-hidden"
              role="progressbar"
              :aria-valuenow="Math.round(m.ratio * 100)"
              aria-valuemin="0"
              aria-valuemax="100"
            >
              <div
                class="h-full rounded-full"
                :class="m.ratio >= 1 ? 'bg-destructive' : m.ratio >= 0.8 ? 'bg-warning' : 'bg-primary'"
                :style="{ width: Math.min(m.ratio, 1) * 100 + '%' }"
              />
            </div>
            <p v-else class="text-xs text-muted-foreground">{{ $t('admin.usage.unlimited') }}</p>
            <p v-if="m.hint" class="text-xs text-muted-foreground">{{ m.hint }}</p>
          </div>
        </div>

        <dl v-if="data" class="grid gap-x-6 gap-y-2 text-sm sm:grid-cols-[max-content_1fr]">
          <dt class="text-muted-foreground">{{ $t('admin.usage.apiRate') }}</dt>
          <dd>{{ data.limits.api_requests_per_minute || $t('admin.usage.unlimited') }}</dd>
          <dt class="text-muted-foreground">{{ $t('admin.usage.throttled') }}</dt>
          <dd class="tabular-nums">{{ data.fair_use.throttled_today }}</dd>
          <dt class="text-muted-foreground">{{ $t('admin.usage.largestUpload') }}</dt>
          <dd>{{ data.limits.file_upload_mb ? data.limits.file_upload_mb + ' MB' : $t('admin.usage.unlimited') }}</dd>
        </dl>
      </div>
    </template>
    <template #help>
      <p>{{ $t('admin.usage.help') }}</p>
    </template>
  </AdminSplitLayout>
</template>

<script setup>
import { ref, computed, onMounted } from 'vue'
import { useI18n } from 'vue-i18n'
import api from '@main/api'
import AdminSplitLayout from '@main/layouts/admin/AdminSplitLayout.vue'
import { EMITTER_EVENTS } from '@main/constants/emitterEvents.js'
import { useEmitter } from '@main/composables/useEmitter'
import { handleHTTPError } from '@shared-ui/utils/http.js'
import { Button } from '@shared-ui/components/ui/button'
import { Badge } from '@shared-ui/components/ui/badge'

const { t } = useI18n()
const emitter = useEmitter()
const data = ref(null)

const size = (b) => {
  const u = ['B', 'KB', 'MB', 'GB', 'TB']
  let i = 0
  while (b >= 1024 && i < u.length - 1) {
    b /= 1024
    i++
  }
  return (i ? b.toFixed(1).replace(/\.0$/, '') : b) + ' ' + u[i]
}

// Each metered resource with what it uses against its limit; a 0 limit is unlimited.
const meters = computed(() => {
  const d = data.value
  const meter = (key, label, used, limit, fmt = (v) => v, hint = '') => ({
    key,
    label,
    used: fmt(used),
    limit,
    limitText: fmt(limit),
    ratio: limit ? used / limit : 0,
    hint
  })
  return [
    meter('agents', t('admin.usage.agents'), d.usage.agents, d.limits.agents),
    meter('inboxes', t('admin.usage.inboxes'), d.usage.inboxes, d.limits.inboxes),
    meter('storage', t('admin.usage.storage'), d.fair_use.storage_bytes, d.limits.storage_mb * 1048576, size),
    meter(
      'ai',
      t('admin.usage.aiRequests'),
      d.fair_use.ai_requests_month,
      d.limits.ai_requests_per_month,
      (v) => v,
      t('admin.usage.aiResets')
    )
  ]
})

onMounted(async () => {
  try {
    data.value = (await api.getWorkspaceUsage()).data.data
  } catch (error) {
    emitter.emit(EMITTER_EVENTS.SHOW_TOAST, { variant: 'destructive', description: handleHTTPError(error).message })
  }
})
</script>
