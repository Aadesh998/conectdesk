<template>
  <section class="space-y-4" :class="{ 'opacity-50 transition-opacity': !loaded }">
    <div class="space-y-1">
      <h3 class="font-medium">{{ $t('dailyReport.title') }}</h3>
      <p class="text-sm text-muted-foreground">{{ $t('dailyReport.description') }}</p>
    </div>

    <div class="flex items-start gap-3">
      <Switch id="daily-report-on" :checked="s.enabled" @update:checked="s.enabled = $event" />
      <Label for="daily-report-on">{{ $t('dailyReport.enabled') }}</Label>
    </div>

    <div class="grid gap-4 sm:grid-cols-2">
      <div class="space-y-1.5">
        <Label>{{ $t('dailyReport.time') }}</Label>
        <Input v-model="s.time" type="time" class="w-36" />
        <p class="text-xs text-muted-foreground">{{ $t('dailyReport.time.description', { tz: timezone }) }}</p>
      </div>
      <div class="space-y-1.5">
        <Label>{{ $t('dailyReport.inbox') }}</Label>
        <select v-model.number="s.whatsapp_inbox_id" class="w-full rounded-md border bg-background px-2 py-2 text-sm">
          <option :value="0">—</option>
          <option v-for="inb in whatsAppInboxes" :key="inb.id" :value="inb.id">{{ inb.name }}</option>
        </select>
      </div>
    </div>

    <template v-if="s.whatsapp_inbox_id">
      <AutoTemplateField
        v-model="template"
        :inbox-id="s.whatsapp_inbox_id"
        :label="$t('dailyReport.template')"
        :hint="$t('dailyReport.template.hint')"
        :fixed-values="fixedValues"
      />
      <SuggestedTemplateButton :inbox-id="s.whatsapp_inbox_id" :template="SUGGESTED_TEMPLATES.dailyReport" @created="template = $event" />
    </template>

    <div class="space-y-1.5">
      <Label>{{ $t('dailyReport.recipients') }}</Label>
      <Input v-model="recipients" placeholder="+91 98000 00001, +91 98000 00002" />
      <p class="text-xs text-muted-foreground">{{ $t('dailyReport.recipients.description') }}</p>
    </div>

    <div class="flex flex-wrap gap-2">
      <Button :disabled="!loaded" :is-loading="saving" @click="save">{{ $t('globals.messages.save') }}</Button>
      <Button variant="outline" :disabled="!loaded || !s.whatsapp_inbox_id" :is-loading="testing" @click="test">
        {{ $t('dailyReport.sendTest') }}
      </Button>
    </div>

    <div v-if="runs.length" class="space-y-1 text-sm">
      <p class="text-xs font-medium text-muted-foreground">{{ $t('dailyReport.recent') }}</p>
      <p v-for="r in runs" :key="r.report_date" class="text-xs">
        <span class="tabular-nums">{{ r.report_date }}</span> ·
        <span :class="r.status === 'failed' ? 'text-destructive' : 'text-emerald-700 dark:text-emerald-400'">
          {{ $t(`dailyReport.status.${r.status}`) }}
        </span>
        <span v-if="r.error" class="text-muted-foreground"> · {{ r.error }}</span>
      </p>
    </div>
  </section>
</template>

<script setup>
import { computed, onMounted, reactive, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import api from '@main/api'
import { Button } from '@shared-ui/components/ui/button'
import { Input } from '@shared-ui/components/ui/input'
import { Label } from '@shared-ui/components/ui/label'
import { Switch } from '@shared-ui/components/ui/switch'
import { handleHTTPError } from '@shared-ui/utils/http.js'
import AutoTemplateField from '@main/features/admin/inbox/AutoTemplateField.vue'
import SuggestedTemplateButton from './SuggestedTemplateButton.vue'
import { SUGGESTED_TEMPLATES } from '@main/constants/suggestedTemplates.js'
import { useInboxStore } from '@main/stores/inbox'
import { useEmitter } from '@main/composables/useEmitter'
import { EMITTER_EVENTS } from '@main/constants/emitterEvents.js'

const { t } = useI18n()
const emitter = useEmitter()
const inboxStore = useInboxStore()
inboxStore.fetchInboxes()
const whatsAppInboxes = computed(() => inboxStore.inboxes.filter((i) => i.channel === 'whatsapp'))

const s = reactive({ enabled: false, time: '20:00', whatsapp_inbox_id: 0 })
const template = ref(null)
const recipients = ref('')
const runs = ref([])
const timezone = ref('')
const loaded = ref(false)
const saving = ref(false)
const testing = ref(false)

// What the report fills in, in the template's order.
const fixedValues = computed(() => [
  t('dailyReport.value.date'),
  t('dailyReport.value.enquiries'),
  t('dailyReport.value.calls'),
  t('dailyReport.value.missed'),
  t('dailyReport.value.waiting'),
  t('dailyReport.value.admissions')
])

const apply = (data) => {
  const v = data.settings || {}
  s.enabled = !!v['daily_report.enabled']
  s.time = v['daily_report.time'] || '20:00'
  s.whatsapp_inbox_id = v['daily_report.whatsapp_inbox_id'] || 0
  template.value = v['daily_report.template_name']
    ? { name: v['daily_report.template_name'], language: v['daily_report.template_language'], body: [] }
    : null
  recipients.value = (v['daily_report.recipients'] || []).join(', ')
  runs.value = data.runs || []
  timezone.value = data.timezone || ''
}

const payload = () => ({
  'daily_report.enabled': s.enabled,
  'daily_report.time': s.time,
  'daily_report.whatsapp_inbox_id': s.whatsapp_inbox_id,
  'daily_report.template_name': template.value?.name || '',
  'daily_report.template_language': template.value?.language || '',
  'daily_report.recipients': recipients.value.split(/[,\n]/).map((n) => n.trim()).filter(Boolean)
})

const toast = (description, variant) => emitter.emit(EMITTER_EVENTS.SHOW_TOAST, { description, variant })

const save = async () => {
  saving.value = true
  try {
    const resp = await api.updateDailyReportSettings(payload())
    apply(resp.data.data)
    toast(t('globals.messages.savedSuccessfully'))
  } catch (error) {
    toast(handleHTTPError(error).message, 'destructive')
  } finally {
    saving.value = false
  }
}

const test = async () => {
  testing.value = true
  try {
    await api.testDailyReport(payload())
    toast(t('dailyReport.testSent'))
  } catch (error) {
    toast(handleHTTPError(error).message, 'destructive')
  } finally {
    testing.value = false
  }
}

onMounted(async () => {
  try {
    const resp = await api.getDailyReportSettings()
    apply(resp.data.data)
  } finally {
    loaded.value = true
  }
})
</script>
