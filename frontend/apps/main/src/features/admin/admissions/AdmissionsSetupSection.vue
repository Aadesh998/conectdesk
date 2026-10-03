<template>
  <section class="space-y-4">
    <div class="space-y-1">
      <h3 class="font-medium">{{ $t('admissionsSetup.title') }}</h3>
      <p class="text-sm text-muted-foreground">{{ $t('admissionsSetup.description') }}</p>
    </div>

    <div class="space-y-2 text-sm">
      <label class="flex items-start gap-2">
        <input v-model="req.fields" type="checkbox" class="mt-0.5 size-4 accent-primary" />
        <span>{{ $t('admissionsSetup.fields') }}</span>
      </label>
      <label class="flex items-start gap-2">
        <input v-model="req.snippets" type="checkbox" class="mt-0.5 size-4 accent-primary" />
        <span>{{ $t('admissionsSetup.snippets') }}</span>
      </label>
      <label class="flex items-start gap-2">
        <input v-model="req.assistant" type="checkbox" class="mt-0.5 size-4 accent-primary" />
        <span>{{ $t('admissionsSetup.assistant') }}</span>
      </label>
    </div>

    <div v-if="req.assistant" class="grid gap-4 pl-6 sm:grid-cols-2">
      <div class="space-y-1.5">
        <Label>{{ $t('admissionsSetup.assistantName') }}</Label>
        <Input v-model="req.assistant_name" />
      </div>
      <div class="space-y-1.5">
        <Label>{{ $t('admissionsSetup.fallbackTeam') }}</Label>
        <select v-model.number="req.fallback_team_id" class="w-full rounded-md border bg-background px-2 py-2 text-sm">
          <option :value="0">—</option>
          <option v-for="team in teamStore.options" :key="team.value" :value="Number(team.value)">{{ team.label }}</option>
        </select>
      </div>
      <div class="space-y-1.5 sm:col-span-2">
        <Label>{{ $t('admissionsSetup.inboxes') }}</Label>
        <p v-if="!whatsAppInboxes.length" class="text-xs text-muted-foreground">{{ $t('admissionsSetup.noWhatsApp') }}</p>
        <label v-for="inb in whatsAppInboxes" :key="inb.id" class="flex items-center gap-2 text-sm">
          <input v-model="req.whatsapp_inbox_ids" type="checkbox" :value="inb.id" class="size-4 accent-primary" />
          {{ inb.name }}
        </label>
      </div>
    </div>

    <Button :is-loading="running" :disabled="!req.fields && !req.snippets && !req.assistant" @click="run">
      {{ $t('admissionsSetup.run') }}
    </Button>

    <div v-if="result" class="box space-y-1 p-3 text-sm">
      <p v-if="result.fields_added.length">{{ $t('admissionsSetup.result.fields', { list: result.fields_added.join(', ') }) }}</p>
      <p v-if="result.snippets_added.length">
        {{ $t('admissionsSetup.result.snippets', { list: result.snippets_added.join(', ') }) }}
        <router-link to="/admin/ai/snippets" class="link-style">{{ $t('admissionsSetup.result.fillSnippets') }}</router-link>
      </p>
      <p v-if="result.assistant_id">
        {{ result.assistant_added ? $t('admissionsSetup.result.assistantAdded') : $t('admissionsSetup.result.assistantExists') }}
        <router-link :to="`/admin/ai/assistants/${result.assistant_id}/edit`" class="link-style">{{ $t('admissionsSetup.result.openAssistant') }}</router-link>
      </p>
      <p v-if="result.rule_added">{{ $t('admissionsSetup.result.rule') }}</p>
      <p v-if="nothingNew" class="text-muted-foreground">{{ $t('admissionsSetup.result.nothing') }}</p>
    </div>
  </section>
</template>

<script setup>
import { computed, reactive, ref } from 'vue'
import api from '@main/api'
import { Button } from '@shared-ui/components/ui/button'
import { Input } from '@shared-ui/components/ui/input'
import { Label } from '@shared-ui/components/ui/label'
import { handleHTTPError } from '@shared-ui/utils/http.js'
import { useInboxStore } from '@main/stores/inbox'
import { useTeamStore } from '@main/stores/team'
import { useCustomAttributeStore } from '@main/stores/customAttributes'
import { useEmitter } from '@main/composables/useEmitter'
import { EMITTER_EVENTS } from '@main/constants/emitterEvents.js'

const emitter = useEmitter()
const inboxStore = useInboxStore()
const teamStore = useTeamStore()
inboxStore.fetchInboxes()
teamStore.fetchTeams()
const whatsAppInboxes = computed(() => inboxStore.inboxes.filter((i) => i.channel === 'whatsapp'))

const req = reactive({
  fields: true,
  snippets: true,
  assistant: true,
  assistant_name: 'Admissions assistant',
  fallback_team_id: 0,
  whatsapp_inbox_ids: []
})
const running = ref(false)
const result = ref(null)
const nothingNew = computed(
  () =>
    result.value &&
    !result.value.fields_added.length &&
    !result.value.snippets_added.length &&
    !result.value.assistant_added &&
    !result.value.rule_added
)

const run = async () => {
  running.value = true
  try {
    const resp = await api.runAdmissionsSetup({ ...req })
    result.value = resp.data.data
    if (result.value.fields_added.length) useCustomAttributeStore().fetchCustomAttributes(true)
  } catch (error) {
    emitter.emit(EMITTER_EVENTS.SHOW_TOAST, { variant: 'destructive', description: handleHTTPError(error).message })
  } finally {
    running.value = false
  }
}
</script>
