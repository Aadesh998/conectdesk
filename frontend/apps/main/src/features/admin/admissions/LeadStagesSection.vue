<template>
  <section class="space-y-4" :class="{ 'opacity-50 transition-opacity': !loaded }">
    <div class="space-y-1">
      <h3 class="font-medium">{{ $t('lead.stages') }}</h3>
      <p class="text-sm text-muted-foreground">{{ $t('lead.stages.description') }}</p>
    </div>

    <div class="space-y-2">
      <div v-for="(s, i) in stages" :key="s._id" class="flex flex-wrap items-center gap-2">
        <div class="flex flex-col">
          <button type="button" class="text-muted-foreground hover:text-foreground disabled:opacity-30" :disabled="i === 0" @click="swap(i, i - 1)">▲</button>
          <button type="button" class="text-muted-foreground hover:text-foreground disabled:opacity-30" :disabled="i === stages.length - 1" @click="swap(i, i + 1)">▼</button>
        </div>
        <Input v-model="s.label" class="w-56" maxlength="50" />
        <select v-model="s.kind" class="h-9 rounded-md border bg-background px-2 text-sm">
          <option value="open">{{ $t('lead.kind.open') }}</option>
          <option value="won">{{ $t('lead.kind.won') }}</option>
          <option value="lost">{{ $t('lead.kind.lost') }}</option>
        </select>
        <span class="w-24 text-xs text-muted-foreground tabular-nums">
          {{ s.key && counts[s.key] ? $t('lead.contactsOnStage', { n: counts[s.key] }) : '' }}
        </span>
        <Button
          type="button"
          variant="ghost"
          size="sm"
          :disabled="stages.length <= 1 || !!(s.key && counts[s.key])"
          :title="s.key && counts[s.key] ? $t('lead.stageHasContacts') : ''"
          @click="stages.splice(i, 1)"
        >
          ✕
        </Button>
      </div>
      <Button type="button" variant="outline" size="sm" :disabled="stages.length >= 12" @click="add">
        {{ $t('lead.addStage') }}
      </Button>
    </div>

    <div class="flex items-start gap-3">
      <Switch id="lead-auto-new" :checked="autoNew" @update:checked="autoNew = $event" />
      <div class="space-y-1">
        <Label for="lead-auto-new">{{ $t('lead.autoNew') }}</Label>
        <p class="text-xs text-muted-foreground">{{ $t('lead.autoNew.description', { stage: stages[0]?.label || '' }) }}</p>
      </div>
    </div>

    <Button :disabled="!loaded" :is-loading="saving" @click="save">{{ $t('globals.messages.save') }}</Button>
  </section>
</template>

<script setup>
import { onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import api from '@main/api'
import { Button } from '@shared-ui/components/ui/button'
import { Input } from '@shared-ui/components/ui/input'
import { Label } from '@shared-ui/components/ui/label'
import { Switch } from '@shared-ui/components/ui/switch'
import { handleHTTPError } from '@shared-ui/utils/http.js'
import { useLeadStore } from '@main/stores/leads'
import { useEmitter } from '@main/composables/useEmitter'
import { EMITTER_EVENTS } from '@main/constants/emitterEvents.js'

const { t } = useI18n()
const emitter = useEmitter()
const leadStore = useLeadStore()
const stages = ref([])
const counts = ref({})
const autoNew = ref(true)
const loaded = ref(false)
const saving = ref(false)
let nextID = 0

const apply = (data) => {
  stages.value = (data.stages || []).map((s) => ({ ...s, _id: nextID++ }))
  counts.value = data.counts || {}
  autoNew.value = !!data.auto_new
}

const add = () => stages.value.splice(Math.max(stages.value.length - 2, 1), 0, { key: '', label: '', kind: 'open', _id: nextID++ })
const swap = (a, b) => {
  const list = stages.value
  ;[list[a], list[b]] = [list[b], list[a]]
}

const save = async () => {
  saving.value = true
  try {
    const resp = await api.updateLeadSettings({
      stages: stages.value.map(({ key, label, kind }) => ({ key, label, kind })),
      auto_new: autoNew.value
    })
    apply(resp.data.data)
    leadStore.fetchStages(true)
    emitter.emit(EMITTER_EVENTS.SHOW_TOAST, { description: t('globals.messages.savedSuccessfully') })
  } catch (error) {
    emitter.emit(EMITTER_EVENTS.SHOW_TOAST, { variant: 'destructive', description: handleHTTPError(error).message })
  } finally {
    saving.value = false
  }
}

onMounted(async () => {
  try {
    const resp = await api.getLeadSettings()
    apply(resp.data.data)
  } finally {
    loaded.value = true
  }
})
</script>
