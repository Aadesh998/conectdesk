<template>
  <AdminSplitLayout>
    <template #content>
      <div class="space-y-6">
        <div class="space-y-3">
          <p class="text-sm text-muted-foreground">{{ $t('admin.export.description') }}</p>
          <Button :disabled="running || busy" :isLoading="busy" @click="start">
            {{ running ? $t('admin.export.running') : $t('admin.export.start') }}
          </Button>
        </div>

        <p v-if="!exports.length" class="text-sm text-muted-foreground">{{ $t('admin.export.none') }}</p>
        <div v-for="e in exports" :key="e.uuid" class="flex flex-wrap items-center justify-between gap-3 rounded-lg border p-4">
          <div class="space-y-1">
            <div class="flex items-center gap-2">
              <span class="font-medium">{{ formatDate(e.created_at) }}</span>
              <Badge :variant="e.status === 'ready' ? 'default' : e.status === 'failed' ? 'destructive' : 'outline'">
                {{ $t('admin.export.' + e.status) }}
              </Badge>
            </div>
            <p class="text-xs text-muted-foreground">
              <span v-if="e.requested_by_email">{{ $t('admin.export.requestedBy', { email: e.requested_by_email }) }}</span>
              <span v-if="e.status === 'ready'"> · {{ size(e.size) }} · {{ $t('admin.export.expires', { date: formatDate(e.expires_at) }) }}</span>
              <span v-if="e.status === 'failed' && e.error"> · {{ e.error }}</span>
            </p>
          </div>
          <a v-if="e.status === 'ready'" :href="`/api/v1/data-exports/${e.uuid}/download`">
            <Button variant="outline" size="sm">{{ $t('admin.export.download') }}</Button>
          </a>
        </div>
      </div>
    </template>
    <template #help>
      <p>{{ $t('admin.export.help') }}</p>
    </template>
  </AdminSplitLayout>
</template>

<script setup>
import { ref, computed, onMounted, onUnmounted } from 'vue'
import { useI18n } from 'vue-i18n'
import api from '@main/api'
import AdminSplitLayout from '@main/layouts/admin/AdminSplitLayout.vue'
import { EMITTER_EVENTS } from '@main/constants/emitterEvents.js'
import { useEmitter } from '@main/composables/useEmitter'
import { handleHTTPError } from '@shared-ui/utils/http.js'
import { Button } from '@shared-ui/components/ui/button'
import { Badge } from '@shared-ui/components/ui/badge'

const { t, locale } = useI18n()
const emitter = useEmitter()
const exports = ref([])
const busy = ref(false)
let timer = null

const running = computed(() => exports.value.some((e) => e.status === 'running'))
const formatDate = (d) => (d ? new Date(d).toLocaleString(locale.value) : '')
const size = (b) => {
  const u = ['B', 'KB', 'MB', 'GB']
  let i = 0
  while (b >= 1024 && i < u.length - 1) {
    b /= 1024
    i++
  }
  return (i ? b.toFixed(1) : b) + ' ' + u[i]
}

const load = async () => {
  try {
    exports.value = (await api.getDataExports()).data.data
  } catch (error) {
    emitter.emit(EMITTER_EVENTS.SHOW_TOAST, { variant: 'destructive', description: handleHTTPError(error).message })
  }
  // Refresh while an export runs.
  clearTimeout(timer)
  if (running.value) timer = setTimeout(load, 3000)
}

const start = async () => {
  busy.value = true
  try {
    await api.startDataExport()
    emitter.emit(EMITTER_EVENTS.SHOW_TOAST, { description: t('admin.export.started') })
    await load()
  } catch (error) {
    emitter.emit(EMITTER_EVENTS.SHOW_TOAST, { variant: 'destructive', description: handleHTTPError(error).message })
  } finally {
    busy.value = false
  }
}

onMounted(load)
onUnmounted(() => clearTimeout(timer))
</script>
