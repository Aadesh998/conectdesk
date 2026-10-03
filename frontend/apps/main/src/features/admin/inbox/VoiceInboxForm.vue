<template>
  <form class="space-y-6 w-full max-w-2xl" @submit.prevent="submit">
    <p class="text-sm text-muted-foreground">{{ $t('voice.help') }}</p>

    <div class="space-y-1.5">
      <Label>{{ $t('globals.terms.name') }}</Label>
      <Input v-model="form.name" :placeholder="$t('voice.namePlaceholder')" />
    </div>

    <div class="box p-4 space-y-4">
      <h3 class="font-semibold">Exotel</h3>
      <p class="text-xs text-muted-foreground">{{ $t('voice.credentialsHint') }}</p>
      <div class="grid gap-4 sm:grid-cols-2">
        <div class="space-y-1.5">
          <Label>{{ $t('voice.accountSid') }}</Label>
          <Input v-model="form.config.account_sid" autocomplete="off" />
        </div>
        <div class="space-y-1.5">
          <Label>{{ $t('voice.region') }}</Label>
          <select v-model="form.config.subdomain" class="w-full rounded-md border bg-background px-2 py-2 text-sm">
            <option value="api.in.exotel.com">{{ $t('voice.region.mumbai') }}</option>
            <option value="api.exotel.com">{{ $t('voice.region.singapore') }}</option>
          </select>
        </div>
        <div class="space-y-1.5">
          <Label>{{ $t('voice.apiKey') }}</Label>
          <Input v-model="form.config.api_key" autocomplete="off" />
        </div>
        <div class="space-y-1.5">
          <Label>{{ $t('voice.apiToken') }}</Label>
          <Input v-model="form.config.access_token" type="password" autocomplete="off" :placeholder="isNew ? '' : '••••••••'" />
        </div>
      </div>
      <div class="space-y-1.5">
        <Label>{{ $t('voice.exophone') }}</Label>
        <Input v-model="form.config.caller_id" placeholder="+91 80 4711 2233" />
        <p class="text-xs text-muted-foreground">{{ $t('voice.exophone.description') }}</p>
      </div>
      <Button v-if="!isNew" type="button" variant="outline" size="sm" :is-loading="testing" @click="test">
        {{ $t('voice.test') }}
      </Button>
    </div>

    <div class="box p-4 space-y-4">
      <SwitchField
        :title="$t('voice.record')"
        :description="$t('voice.record.description')"
        :checked="form.config.record"
        @update:checked="form.config.record = $event"
      />
      <SwitchField
        :title="$t('voice.ringAgents')"
        :description="$t('voice.ringAgents.description')"
        :checked="form.config.ring_agents"
        @update:checked="form.config.ring_agents = $event"
      />
      <div class="space-y-1.5">
        <Label>{{ $t('voice.fallback') }}</Label>
        <Input v-model="fallback" placeholder="+91 98000 00001, +91 98000 00002" />
        <p class="text-xs text-muted-foreground">{{ $t('voice.fallback.description') }}</p>
      </div>
      <div class="space-y-1.5">
        <Label>{{ $t('voice.ringSeconds') }}</Label>
        <Input v-model.number="form.config.ring_seconds" type="number" min="10" max="120" class="w-28" />
        <p class="text-xs text-muted-foreground">{{ $t('voice.ringSeconds.description') }}</p>
      </div>
      <SwitchField
        :title="$t('globals.terms.enabled')"
        :description="$t('admin.inbox.enabled.description')"
        :checked="form.enabled"
        @update:checked="form.enabled = $event"
      />
    </div>

    <!-- "Press 1 for sales": the digit routes the call to a team. -->
    <div class="box p-4 space-y-3">
      <h3 class="font-semibold">{{ $t('voice.menu') }}</h3>
      <p class="text-xs text-muted-foreground">{{ $t('voice.menu.description') }}</p>
      <div v-for="(o, i) in form.config.menu" :key="i" class="flex items-center gap-2">
        <select v-model="o.digit" class="h-9 rounded-md border bg-background px-2 text-sm">
          <option v-for="d in '1234567890'.split('')" :key="d" :value="d">{{ d }}</option>
        </select>
        <select v-model.number="o.team_id" class="h-9 flex-1 rounded-md border bg-background px-2 text-sm">
          <option v-for="team in teamStore.options" :key="team.value" :value="Number(team.value)">{{ team.label }}</option>
        </select>
        <Input v-model="o.label" class="flex-1" :placeholder="$t('voice.menu.label')" />
        <Button type="button" variant="ghost" size="sm" @click="form.config.menu.splice(i, 1)">✕</Button>
      </div>
      <Button type="button" variant="outline" size="sm" :disabled="form.config.menu.length >= 10 || !teamStore.options.length" @click="addOption">
        {{ $t('voice.menu.add') }}
      </Button>
    </div>

    <!-- What an unanswered call leads to: a call-back for the agents, and a
         WhatsApp message to the caller. -->
    <div class="box p-4 space-y-4">
      <div>
        <h3 class="font-semibold">{{ $t('voice.missedCalls') }}</h3>
        <p class="text-xs text-muted-foreground">{{ $t('voice.missedCalls.description') }}</p>
      </div>
      <SwitchField
        :title="$t('voice.missedCalls.callBacks')"
        :description="$t('voice.missedCalls.callBacks.description')"
        :checked="!form.config.missed_calls.no_call_backs"
        @update:checked="form.config.missed_calls.no_call_backs = !$event"
      />
      <template v-if="!form.config.missed_calls.no_call_backs">
        <div class="space-y-1.5">
          <Label>{{ $t('voice.missedCalls.minutes') }}</Label>
          <Input v-model.number="form.config.missed_calls.call_back_minutes" type="number" min="1" max="1440" class="w-28" />
          <p class="text-xs text-muted-foreground">{{ $t('voice.missedCalls.minutes.description') }}</p>
        </div>
        <div class="space-y-1.5">
          <Label>{{ $t('voice.missedCalls.whatsapp') }}</Label>
          <select
            v-model.number="form.config.missed_calls.whatsapp_inbox_id"
            class="w-full rounded-md border bg-background px-2 py-2 text-sm"
          >
            <option :value="0">{{ $t('voice.missedCalls.whatsapp.off') }}</option>
            <option v-for="inb in whatsAppInboxes" :key="inb.id" :value="inb.id">{{ inb.name }}</option>
          </select>
          <p class="text-xs text-muted-foreground">{{ $t('voice.missedCalls.whatsapp.description') }}</p>
        </div>
        <template v-if="form.config.missed_calls.whatsapp_inbox_id">
          <AutoTemplateField
            v-model="form.config.missed_calls.template"
            :inbox-id="form.config.missed_calls.whatsapp_inbox_id"
            :label="$t('voice.missedCalls.template')"
            :hint="$t('voice.missedCalls.template.hint')"
            placeholder="{{name}}"
          />
          <SuggestedTemplateButton
            :inbox-id="form.config.missed_calls.whatsapp_inbox_id"
            :template="SUGGESTED_TEMPLATES.missedCall"
            @created="form.config.missed_calls.template = $event"
          />
          <AutoTemplateField
            v-model="form.config.missed_calls.after_hours_template"
            :inbox-id="form.config.missed_calls.whatsapp_inbox_id"
            :label="$t('voice.missedCalls.afterHours')"
            :hint="$t('voice.missedCalls.afterHours.hint')"
            placeholder="{{name}}"
            optional
          />
          <SuggestedTemplateButton
            :inbox-id="form.config.missed_calls.whatsapp_inbox_id"
            :template="SUGGESTED_TEMPLATES.missedCallAfterHours"
            @created="form.config.missed_calls.after_hours_template = $event"
          />
        </template>
      </template>
    </div>

    <!-- The flow to build in Exotel's App Bazaar for incoming calls. -->
    <div v-if="setup" class="box p-4 space-y-3">
      <h3 class="font-semibold">{{ $t('voice.setup') }}</h3>
      <ol class="list-decimal space-y-2 pl-5 text-sm">
        <li>{{ $t('voice.setup.step1') }}</li>
        <li v-if="form.config.menu.length">{{ $t('voice.setup.stepMenu') }}</li>
        <li>
          {{ $t('voice.setup.step2') }}
          <UrlRow :url="setup.incoming" />
        </li>
        <li>
          {{ $t('voice.setup.step3') }}
          <UrlRow :url="setup.dial" />
        </li>
        <li>
          {{ $t('voice.setup.step4') }}
          <UrlRow :url="setup.completed" />
        </li>
        <li>
          {{ $t('voice.setup.stepVoicemail') }}
          <UrlRow :url="setup.voicemail" />
        </li>
        <li>{{ $t('voice.setup.step5') }}</li>
      </ol>
      <p class="text-xs text-muted-foreground">{{ $t('voice.setup.secret') }}</p>
    </div>
    <p v-else-if="isNew" class="text-sm text-muted-foreground">{{ $t('voice.setup.afterSave') }}</p>

    <Button type="submit" :is-loading="isLoading" :disabled="isLoading">
      {{ isNew ? $t('globals.messages.create') : $t('globals.messages.save') }}
    </Button>
  </form>
</template>

<script setup>
import { computed, defineComponent, h, reactive, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { Button } from '@shared-ui/components/ui/button'
import { Input } from '@shared-ui/components/ui/input'
import { Label } from '@shared-ui/components/ui/label'
import SwitchField from '@shared-ui/components/SwitchField.vue'
import AutoTemplateField from './AutoTemplateField.vue'
import SuggestedTemplateButton from '@main/features/admin/admissions/SuggestedTemplateButton.vue'
import { SUGGESTED_TEMPLATES } from '@main/constants/suggestedTemplates.js'
import { useInboxStore } from '@/stores/inbox'
import CopyButton from '@/components/button/CopyButton.vue'
import { handleHTTPError } from '@shared-ui/utils/http.js'
import api from '@/api'
import { useTeamStore } from '@/stores/team'
import { useEmitter } from '@main/composables/useEmitter'
import { EMITTER_EVENTS } from '@main/constants/emitterEvents.js'

const props = defineProps({
  initialValues: { type: Object, default: () => ({}) },
  submitForm: { type: Function, required: true },
  isLoading: { type: Boolean, default: false }
})

const UrlRow = defineComponent({
  props: { url: { type: String, required: true } },
  setup: (p) => () =>
    h('div', { class: 'mt-1 flex items-center gap-1.5' }, [
      h('code', { class: 'flex-1 break-all rounded bg-muted px-2 py-1 font-mono text-xs' }, p.url),
      h(CopyButton, { text: p.url })
    ])
})

const { t } = useI18n()
const emitter = useEmitter()
const isNew = computed(() => !props.initialValues?.id)
const setup = ref(null)
const testing = ref(false)
const fallback = ref('')

const form = reactive({
  name: '',
  enabled: true,
  config: {
    provider: 'exotel',
    account_sid: '',
    api_key: '',
    access_token: '',
    subdomain: 'api.in.exotel.com',
    caller_id: '',
    record: true,
    ring_agents: true,
    fallback_numbers: [],
    menu: [],
    ring_seconds: 30,
    missed_calls: emptyMissedCalls()
  }
})

function emptyMissedCalls () {
  const tmpl = () => ({ name: '', language: '', body: [] })
  return { no_call_backs: false, call_back_minutes: 15, whatsapp_inbox_id: 0, template: tmpl(), after_hours_template: tmpl() }
}

const inboxStore = useInboxStore()
inboxStore.fetchInboxes()
const whatsAppInboxes = computed(() => inboxStore.inboxes.filter((i) => i.channel === 'whatsapp'))

watch(
  () => props.initialValues,
  (v) => {
    if (!v || !v.id) return
    form.name = v.name
    form.enabled = v.enabled
    Object.assign(form.config, v.config || {})
    form.config.menu = (v.config?.menu || []).map((o) => ({ ...o }))
    form.config.ring_seconds = v.config?.ring_seconds || 30
    const mc = v.config?.missed_calls || {}
    form.config.missed_calls = {
      ...emptyMissedCalls(),
      ...mc,
      call_back_minutes: mc.call_back_minutes || 15,
      template: { name: '', language: '', body: [], ...(mc.template || {}) },
      after_hours_template: { name: '', language: '', body: [], ...(mc.after_hours_template || {}) }
    }
    // The token never comes back; blank keeps the stored one.
    form.config.access_token = ''
    fallback.value = (v.config?.fallback_numbers || []).join(', ')
    loadSetup(v.id)
  },
  { immediate: true, deep: true }
)

const teamStore = useTeamStore()
teamStore.fetchTeams()

const addOption = () => {
  const used = new Set(form.config.menu.map((o) => o.digit))
  const digit = '1234567890'.split('').find((d) => !used.has(d))
  form.config.menu.push({ digit, team_id: Number(teamStore.options[0]?.value), label: '' })
}

const loadSetup = async (id) => {
  try {
    const resp = await api.getVoiceSetup(id)
    setup.value = resp.data.data
  } catch {
    setup.value = null
  }
}

const submit = () => {
  props.submitForm({
    name: form.name,
    enabled: form.enabled,
    channel: 'voice',
    config: {
      ...form.config,
      fallback_numbers: fallback.value
        .split(',')
        .map((n) => n.trim())
        .filter(Boolean)
    }
  })
}

const test = async () => {
  testing.value = true
  try {
    await api.testVoice(props.initialValues.id)
    emitter.emit(EMITTER_EVENTS.SHOW_TOAST, { description: t('voice.testOk') })
  } catch (error) {
    emitter.emit(EMITTER_EVENTS.SHOW_TOAST, { variant: 'destructive', description: handleHTTPError(error).message })
  } finally {
    testing.value = false
  }
}
</script>
