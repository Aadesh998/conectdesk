<template>
  <div class="overflow-y-auto">
    <div class="p-4 md:p-6 w-full space-y-6" :class="{ 'opacity-50 transition-opacity duration-300': loading }">
      <div class="flex flex-wrap items-center justify-between gap-3">
        <h2 class="text-lg font-semibold">{{ $t('reports.calls') }}</h2>
        <div class="flex flex-wrap gap-1">
          <Button
            v-for="d in ranges"
            :key="d"
            size="sm"
            :variant="days === d ? 'default' : 'outline'"
            :disabled="loading"
            @click="days = d"
          >
            {{ d === 0 ? $t('reports.calls.today') : $t('globals.messages.nDays', { days: d }) }}
          </Button>
        </div>
      </div>

      <!-- Totals -->
      <div class="grid grid-cols-2 gap-3 md:grid-cols-4 lg:grid-cols-7">
        <div v-for="tile in tiles" :key="tile.key" class="box p-4">
          <p class="text-xs text-muted-foreground">{{ tile.label }}</p>
          <p class="mt-1 text-2xl font-semibold tabular-nums">{{ tile.value }}</p>
        </div>
      </div>

      <!-- Calls per day -->
      <div class="box p-4 space-y-3">
        <h3 class="font-semibold">{{ $t('reports.calls.perDay') }}</h3>
        <p v-if="!stats.daily.length" class="text-sm text-muted-foreground">{{ $t('reports.calls.none') }}</p>
        <div v-else class="space-y-1.5">
          <div v-for="d in stats.daily" :key="d.date" class="flex items-center gap-3 text-sm">
            <span class="w-24 shrink-0 text-muted-foreground tabular-nums">{{ d.date }}</span>
            <div class="flex h-4 flex-1 overflow-hidden rounded bg-muted">
              <div class="bg-sky-500" :style="{ width: pct(d.inbound - d.missed) }" :title="$t('reports.calls.answeredIn')" />
              <div class="bg-destructive/70" :style="{ width: pct(d.missed) }" :title="$t('reports.calls.missed')" />
              <div class="bg-emerald-500" :style="{ width: pct(d.outbound) }" :title="$t('reports.calls.outbound')" />
            </div>
            <span class="w-28 shrink-0 text-right text-xs text-muted-foreground tabular-nums">
              {{ d.inbound }} {{ $t('reports.calls.in') }} · {{ d.outbound }} {{ $t('reports.calls.out') }}
            </span>
          </div>
          <div class="flex gap-4 pt-2 text-xs text-muted-foreground">
            <span class="flex items-center gap-1"><span class="size-2 rounded-sm bg-sky-500" />{{ $t('reports.calls.answeredIn') }}</span>
            <span class="flex items-center gap-1"><span class="size-2 rounded-sm bg-destructive/70" />{{ $t('reports.calls.missed') }}</span>
            <span class="flex items-center gap-1"><span class="size-2 rounded-sm bg-emerald-500" />{{ $t('reports.calls.outbound') }}</span>
          </div>
        </div>
      </div>

      <!-- Per agent -->
      <div class="box p-4 space-y-3">
        <h3 class="font-semibold">{{ $t('reports.calls.byAgent') }}</h3>
        <p v-if="!stats.agents.length" class="text-sm text-muted-foreground">{{ $t('reports.calls.none') }}</p>
        <table v-else class="w-full text-sm">
          <thead class="text-left text-xs text-muted-foreground">
            <tr>
              <th class="py-1 font-medium">{{ $t('globals.terms.agent') }}</th>
              <th class="py-1 font-medium text-right">{{ $t('reports.calls.total') }}</th>
              <th class="py-1 font-medium text-right">{{ $t('reports.calls.answered') }}</th>
              <th class="py-1 font-medium text-right">{{ $t('reports.calls.talkTime') }}</th>
            </tr>
          </thead>
          <tbody>
            <tr v-for="a in stats.agents" :key="a.agent_id" class="border-t">
              <td class="py-1.5">{{ a.name || '—' }}</td>
              <td class="py-1.5 text-right tabular-nums">{{ a.calls }}</td>
              <td class="py-1.5 text-right tabular-nums">{{ a.answered }}</td>
              <td class="py-1.5 text-right tabular-nums">{{ duration(a.talk_seconds) }}</td>
            </tr>
          </tbody>
        </table>
      </div>

      <!-- Call log: every call, where it was taken and its recording -->
      <div class="box p-4 space-y-3">
        <div class="flex flex-wrap items-center justify-between gap-2">
          <h3 class="font-semibold">{{ $t('reports.calls.log') }}</h3>
          <label class="flex items-center gap-2 text-sm text-muted-foreground">
            <input v-model="recordedOnly" type="checkbox" class="size-4 accent-primary" />
            {{ $t('reports.calls.log.recordedOnly') }}
          </label>
        </div>
        <p v-if="!log.results.length" class="text-sm text-muted-foreground">{{ $t('reports.calls.none') }}</p>
        <div v-else class="overflow-x-auto">
          <table class="w-full text-sm">
            <thead class="text-left text-xs text-muted-foreground">
              <tr>
                <th class="py-1 pr-3 font-medium">{{ $t('reports.calls.log.when') }}</th>
                <th class="py-1 pr-3 font-medium">{{ $t('reports.calls.log.contact') }}</th>
                <th class="py-1 pr-3 font-medium">{{ $t('reports.calls.log.call') }}</th>
                <th class="py-1 pr-3 font-medium">{{ $t('globals.terms.agent') }}</th>
                <th class="py-1 pr-3 font-medium text-right">{{ $t('reports.calls.log.duration') }}</th>
                <th class="py-1 font-medium">{{ $t('reports.calls.log.recording') }}</th>
              </tr>
            </thead>
            <tbody>
              <tr v-for="row in log.results" :key="row.message_uuid" class="border-t align-middle">
                <td class="py-2 pr-3 whitespace-nowrap text-muted-foreground tabular-nums">
                  {{ formatWhen(row.created_at) }}
                </td>
                <td class="py-2 pr-3">
                  <router-link
                    :to="{ name: 'inbox-conversation', params: { type: 'all', uuid: row.conversation_uuid } }"
                    class="hover:underline"
                  >
                    {{ row.contact_name || callNumber(row.call) }}
                  </router-link>
                  <p class="text-xs text-muted-foreground">
                    {{ callNumber(row.call) }} · #{{ row.reference_number }}
                  </p>
                </td>
                <td class="py-2 pr-3 whitespace-nowrap">
                  <span class="inline-flex items-center gap-1.5">
                    <component :is="callIcon(row.call)" :size="14" :class="isMissed(row.call) ? 'text-destructive' : 'text-muted-foreground'" />
                    {{ callLabel(row.call) }}
                  </span>
                </td>
                <td class="py-2 pr-3">{{ row.call?.agent_name || '—' }}</td>
                <td class="py-2 pr-3 text-right tabular-nums">
                  {{ row.call?.duration ? clock(row.call.duration) : '—' }}
                </td>
                <td class="py-2">
                  <div v-if="row.recording_url" class="flex items-center gap-2">
                    <audio :src="row.recording_url" controls preload="none" class="h-8 w-56 max-w-full" />
                    <a
                      :href="row.recording_url"
                      :download="row.recording_name || 'recording.mp3'"
                      class="text-muted-foreground hover:text-foreground"
                      :title="$t('reports.calls.log.download')"
                    >
                      <Download :size="16" />
                    </a>
                  </div>
                  <span v-else class="text-xs text-muted-foreground">{{ $t('reports.calls.log.noRecording') }}</span>
                </td>
              </tr>
            </tbody>
          </table>
        </div>
        <div v-if="pages > 1" class="flex items-center justify-end gap-2 text-sm">
          <span class="text-muted-foreground">{{ $t('reports.calls.log.page', { page, pages }) }}</span>
          <Button size="sm" variant="outline" :disabled="page <= 1 || logLoading" @click="page--">
            {{ $t('reports.calls.log.prev') }}
          </Button>
          <Button size="sm" variant="outline" :disabled="page >= pages || logLoading" @click="page++">
            {{ $t('reports.calls.log.next') }}
          </Button>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup>
import { computed, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { Button } from '@shared-ui/components/ui/button'
import { handleHTTPError } from '@shared-ui/utils/http.js'
import api from '@main/api'
import { useEmitter } from '@main/composables/useEmitter'
import { EMITTER_EVENTS } from '@main/constants/emitterEvents.js'
import { Download, PhoneIncoming, PhoneMissed, PhoneOutgoing } from 'lucide-vue-next'

const { t } = useI18n()
const emitter = useEmitter()
const ranges = [0, 7, 30, 90]
const days = ref(7)
const loading = ref(false)
const stats = ref({ totals: {}, daily: [], agents: [] })

const duration = (s) => {
  s = s || 0
  const h = Math.floor(s / 3600)
  const m = Math.floor(s / 60) % 60
  return h ? `${h}h ${m}m` : `${m}m ${s % 60}s`
}

const maxDay = computed(() => Math.max(1, ...stats.value.daily.map((d) => d.inbound + d.outbound)))
const pct = (n) => `${Math.max(0, (n / maxDay.value) * 100)}%`

const tiles = computed(() => {
  const tt = stats.value.totals || {}
  const missedRate = tt.inbound ? Math.round((tt.missed / tt.inbound) * 100) : 0
  return [
    { key: 'calls', label: t('reports.calls.total'), value: tt.calls || 0 },
    { key: 'inbound', label: t('reports.calls.inbound'), value: tt.inbound || 0 },
    { key: 'outbound', label: t('reports.calls.outbound'), value: tt.outbound || 0 },
    { key: 'missed', label: t('reports.calls.missed'), value: tt.missed || 0 },
    { key: 'missedRate', label: t('reports.calls.missedRate'), value: `${missedRate}%` },
    { key: 'voicemails', label: t('reports.calls.voicemails'), value: tt.voicemails || 0 },
    { key: 'avg', label: t('reports.calls.avgDuration'), value: duration(tt.avg_seconds) }
  ]
})

const load = async () => {
  loading.value = true
  try {
    const resp = await api.getCallStats(days.value)
    stats.value = { totals: {}, daily: [], agents: [], ...resp.data.data }
  } catch (error) {
    emitter.emit(EMITTER_EVENTS.SHOW_TOAST, { variant: 'destructive', description: handleHTTPError(error).message })
  } finally {
    loading.value = false
  }
}

// The call log, a page at a time.
const pageSize = 25
const page = ref(1)
const recordedOnly = ref(false)
const logLoading = ref(false)
const log = ref({ results: [], total: 0 })
const pages = computed(() => Math.max(1, Math.ceil(log.value.total / pageSize)))

const loadLog = async () => {
  logLoading.value = true
  try {
    const resp = await api.getCallLog({
      days: days.value,
      page: page.value,
      page_size: pageSize,
      recorded: recordedOnly.value
    })
    log.value = { results: [], total: 0, ...resp.data.data }
  } catch (error) {
    emitter.emit(EMITTER_EVENTS.SHOW_TOAST, { variant: 'destructive', description: handleHTTPError(error).message })
  } finally {
    logLoading.value = false
  }
}

const missedStatuses = ['no-answer', 'busy', 'failed', 'voicemail']
const isMissed = (call) => missedStatuses.includes(call?.status)
const callIcon = (call) => {
  if (isMissed(call)) return PhoneMissed
  return call?.direction === 'inbound' ? PhoneIncoming : PhoneOutgoing
}
const callLabel = (call) =>
  call?.direction && call?.status ? t(`voice.call.${call.direction}.${call.status}`) : '—'
const callNumber = (call) => (call?.direction === 'inbound' ? call?.from : call?.to) || ''
const clock = (s) => {
  const pad = (n) => String(n).padStart(2, '0')
  return s >= 3600 ? `${Math.floor(s / 3600)}:${pad(Math.floor(s / 60) % 60)}:${pad(s % 60)}` : `${Math.floor(s / 60)}:${pad(s % 60)}`
}
const formatWhen = (ts) =>
  new Date(ts).toLocaleString(undefined, { dateStyle: 'medium', timeStyle: 'short' })

watch(days, load, { immediate: true })
watch([days, recordedOnly], () => {
  if (page.value !== 1) page.value = 1
  else loadLog()
}, { immediate: true })
watch(page, loadLog)
</script>
