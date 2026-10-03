<template>
  <!-- Where a contact is in the admissions pipeline, and moving them on. -->
  <div v-if="contactId && leadStore.stages.length" class="flex items-center gap-2">
    <KanbanSquare size="16" class="text-muted-foreground flex-shrink-0" />
    <select
      :value="stage"
      class="h-7 min-w-0 flex-1 rounded-md border bg-background px-1.5 text-xs"
      :class="stageClass"
      :disabled="saving || !userStore.can('contacts:write')"
      :title="since"
      @change="save($event.target.value)"
    >
      <option value="">{{ $t('lead.notALead') }}</option>
      <option v-for="s in leadStore.stages" :key="s.key" :value="s.key">{{ s.label }}</option>
    </select>
  </div>
</template>

<script setup>
import { computed, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { KanbanSquare } from 'lucide-vue-next'
import { getRelativeTime } from '@shared-ui/utils/datetime.js'
import { handleHTTPError } from '@shared-ui/utils/http.js'
import api from '@main/api'
import { useLeadStore } from '@main/stores/leads'
import { useUserStore } from '@main/stores/user'
import { useEmitter } from '@main/composables/useEmitter'
import { EMITTER_EVENTS } from '@main/constants/emitterEvents.js'

const props = defineProps({
  contactId: { type: [Number, String], default: 0 }
})

const { t } = useI18n()
const emitter = useEmitter()
const leadStore = useLeadStore()
const userStore = useUserStore()
const stage = ref('')
const changedAt = ref(null)
const saving = ref(false)

const kind = computed(() => leadStore.stageOf(stage.value)?.kind)
const stageClass = computed(
  () =>
    ({
      won: 'border-emerald-500/60 text-emerald-700 dark:text-emerald-400',
      lost: 'text-muted-foreground'
    })[kind.value] || ''
)
const since = computed(() =>
  changedAt.value ? t('lead.since', { time: getRelativeTime(new Date(changedAt.value)) }) : ''
)

const load = async () => {
  if (!props.contactId) return
  leadStore.fetchStages()
  try {
    const resp = await api.getContactLead(props.contactId)
    stage.value = resp.data.data?.stage || ''
    changedAt.value = resp.data.data?.stage_changed_at || null
  } catch {
    stage.value = ''
  }
}

const save = async (value) => {
  const before = stage.value
  stage.value = value
  saving.value = true
  try {
    const resp = await api.setContactLead(props.contactId, value)
    changedAt.value = resp.data.data?.stage_changed_at || null
  } catch (error) {
    stage.value = before
    emitter.emit(EMITTER_EVENTS.SHOW_TOAST, { variant: 'destructive', description: handleHTTPError(error).message })
  } finally {
    saving.value = false
  }
}

watch(() => props.contactId, load, { immediate: true })
// Another agent, or the app itself, moved this contact.
watch(
  () => leadStore.lastChange,
  (c) => {
    if (c && Number(c.contact_id) === Number(props.contactId)) load()
  }
)
</script>
