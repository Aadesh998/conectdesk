<template>
  <!-- Today's admissions numbers so far, the same as the owner's daily report. -->
  <div v-if="summary" class="space-y-2">
    <h3 class="text-sm font-semibold">{{ $t('dailyReport.today') }}</h3>
    <div class="grid grid-cols-2 gap-3 sm:grid-cols-3 lg:grid-cols-6">
      <div v-for="tile in tiles" :key="tile.key" class="box p-3">
        <p class="text-xs text-muted-foreground">{{ tile.label }}</p>
        <p class="mt-1 text-2xl font-semibold tabular-nums" :class="tile.class">{{ tile.value }}</p>
      </div>
    </div>
  </div>
</template>

<script setup>
import { computed, onMounted, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import api from '@main/api'

const props = defineProps({
  // Changes to reload, e.g. the overview's refresh count.
  refreshKey: { type: Number, default: 0 }
})

const { t } = useI18n()
const summary = ref(null)

const load = async () => {
  try {
    const resp = await api.getTodaySummary()
    summary.value = resp.data.data
  } catch {
    summary.value = null
  }
}

const tiles = computed(() => {
  const s = summary.value || {}
  return [
    { key: 'enquiries', label: t('dailyReport.value.enquiries'), value: s.enquiries },
    { key: 'calls', label: t('dailyReport.value.calls'), value: s.calls_in },
    { key: 'missed', label: t('dailyReport.value.missed'), value: s.missed_calls },
    {
      key: 'waiting',
      label: t('dailyReport.value.waiting'),
      value: s.pending_call_backs,
      class: s.overdue_call_backs ? 'text-destructive' : ''
    },
    { key: 'called', label: t('dailyReport.value.calledBack'), value: s.called_back },
    { key: 'admissions', label: t('dailyReport.value.admissions'), value: s.admissions, class: 'text-emerald-600' }
  ]
})

onMounted(load)
watch(() => props.refreshKey, load)
</script>
