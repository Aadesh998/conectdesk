<template>
  <!-- A phone call logged in the conversation, updated as it rings and ends. -->
  <div class="mb-1 space-y-2 text-sm">
    <div class="flex items-center gap-2">
      <component :is="icon" :size="16" :class="iconClass" />
      <span class="font-medium">{{ title }}</span>
      <span v-if="call.duration" class="text-muted-foreground">· {{ duration }}</span>
    </div>
    <p class="text-xs text-muted-foreground">
      {{ call.direction === 'inbound' ? call.from : call.to }}
      <template v-if="call.agent_name"> · {{ call.agent_name }}</template>
      <template v-if="call.menu"> · {{ call.menu }}</template>
    </p>
    <p v-if="call.transferred_from" class="text-xs text-muted-foreground">
      {{ $t('voice.transferredFrom', { name: call.transferred_from }) }}
    </p>
    <!-- Logged from Exotel's call list: the flow didn't report it. -->
    <p v-if="call.recovered" class="text-xs text-muted-foreground">{{ $t('voice.recovered') }}</p>
    <audio v-if="recording" :src="recording.url" controls preload="none" class="w-full max-w-xs" />
    <p v-else-if="call.recording_error" class="text-xs text-muted-foreground">
      {{ $t('voice.recordingUnavailable') }}
    </p>

    <!-- What was said, transcribed and summarised by AI. -->
    <div v-if="ai?.status === 'working'" class="flex items-center gap-1.5 text-xs text-muted-foreground">
      <Sparkles :size="12" class="animate-pulse" /> {{ $t('callAI.working') }}
    </div>
    <div v-else-if="ai?.status === 'done'" class="space-y-1.5 rounded-md border bg-muted/30 p-2">
      <p class="flex items-center gap-1 text-xs font-medium text-muted-foreground">
        <Sparkles :size="12" /> {{ $t('callAI.summary') }}
      </p>
      <ul class="list-disc space-y-0.5 pl-4 text-xs">
        <li v-for="(line, i) in summaryLines" :key="i">{{ line }}</li>
      </ul>
      <p v-if="filledText" class="text-xs text-muted-foreground">{{ filledText }}</p>
      <details v-if="ai.transcript" class="text-xs">
        <summary class="cursor-pointer text-muted-foreground hover:text-foreground">{{ $t('callAI.transcript') }}</summary>
        <p class="mt-1 whitespace-pre-wrap">{{ ai.transcript }}</p>
      </details>
    </div>
    <div v-else-if="recording && canSummarize" class="flex flex-wrap items-center gap-2">
      <p v-if="ai?.status === 'failed'" class="text-xs text-destructive">{{ $t('callAI.failed', { error: ai.error }) }}</p>
      <Button type="button" variant="outline" size="sm" class="h-7 gap-1 text-xs" :is-loading="starting" @click="summarize">
        <Sparkles :size="12" /> {{ ai?.status === 'failed' ? $t('callAI.retry') : $t('callAI.summarize') }}
      </Button>
    </div>
  </div>
</template>

<script setup>
import { computed, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { PhoneIncoming, PhoneMissed, PhoneOutgoing, PhoneCall, Sparkles } from 'lucide-vue-next'
import { Button } from '@shared-ui/components/ui/button'
import { handleHTTPError } from '@shared-ui/utils/http.js'
import api from '@main/api'
import { useUserStore } from '@main/stores/user'
import { useEmitter } from '@main/composables/useEmitter'
import { EMITTER_EVENTS } from '@main/constants/emitterEvents.js'

const props = defineProps({
  call: { type: Object, required: true },
  // The call's AI transcript and summary: { status, summary, transcript, error, filled }.
  ai: { type: Object, default: null },
  attachments: { type: Array, default: () => [] },
  messageUuid: { type: String, default: '' },
  conversationUuid: { type: String, default: '' }
})

const { t } = useI18n()
const emitter = useEmitter()
const userStore = useUserStore()
const starting = ref(false)

const canSummarize = computed(() => !!props.messageUuid && !!props.conversationUuid && userStore.can('messages:write'))
const summaryLines = computed(() =>
  (props.ai?.summary || '')
    .split('\n')
    .map((l) => l.replace(/^\s*[-*•]\s*/, '').trim())
    .filter(Boolean)
)
const filledText = computed(() => {
  const keys = Object.keys(props.ai?.filled || {})
  return keys.length ? t('callAI.filled', { fields: keys.join(', ') }) : ''
})

const summarize = async () => {
  starting.value = true
  try {
    await api.summarizeCall(props.conversationUuid, props.messageUuid)
  } catch (error) {
    emitter.emit(EMITTER_EVENTS.SHOW_TOAST, { variant: 'destructive', description: handleHTTPError(error).message })
  } finally {
    starting.value = false
  }
}

const missed = computed(() => ['no-answer', 'busy', 'failed', 'voicemail'].includes(props.call.status))
const icon = computed(() => {
  if (props.call.status === 'ringing') return PhoneCall
  if (missed.value) return PhoneMissed
  return props.call.direction === 'inbound' ? PhoneIncoming : PhoneOutgoing
})
const iconClass = computed(() =>
  missed.value ? 'text-destructive' : props.call.status === 'ringing' ? 'text-amber-500 animate-pulse' : 'text-success'
)
const title = computed(() => t(`voice.call.${props.call.direction}.${props.call.status}`))
const duration = computed(() => {
  const s = props.call.duration || 0
  const pad = (n) => String(n).padStart(2, '0')
  return s >= 3600 ? `${Math.floor(s / 3600)}:${pad(Math.floor(s / 60) % 60)}:${pad(s % 60)}` : `${Math.floor(s / 60)}:${pad(s % 60)}`
})
const recording = computed(() => props.attachments.find((a) => (a.content_type || '').startsWith('audio/')))
</script>
