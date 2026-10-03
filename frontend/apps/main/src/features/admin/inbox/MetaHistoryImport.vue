<template>
  <div class="box p-4 space-y-3">
    <h3 class="font-semibold">{{ $t('inbox.history') }}</h3>
    <p class="text-sm text-muted-foreground">{{ $t('inbox.history.description') }}</p>

    <div class="grid grid-cols-2 gap-4">
      <div class="space-y-1.5">
        <Label>{{ $t('inbox.history.conversations') }}</Label>
        <Input v-model="opts.conversation_limit" type="number" min="1" max="500" />
      </div>
      <div class="space-y-1.5">
        <Label>{{ $t('inbox.history.messages') }}</Label>
        <Input v-model="opts.message_limit" type="number" min="1" max="200" />
      </div>
    </div>
    <label class="flex items-center gap-2 text-sm">
      <input v-model="opts.resolve" type="checkbox" />
      {{ $t('inbox.history.resolve') }}
    </label>

    <Button type="button" variant="outline" :is-loading="running" :disabled="running" @click="start">
      {{ $t('inbox.history.start') }}
    </Button>

    <div v-if="job" class="text-sm space-y-1">
      <p v-if="job.running" class="text-muted-foreground">
        {{ $t('inbox.history.running', { done: job.success }) }}
      </p>
      <p v-for="(line, i) in job.logs" :key="i" class="text-xs text-muted-foreground">{{ line }}</p>
    </div>
  </div>
</template>

<script setup>
import { onUnmounted, reactive, ref } from 'vue'
import { Button } from '@shared-ui/components/ui/button'
import { Input } from '@shared-ui/components/ui/input'
import { Label } from '@shared-ui/components/ui/label'
import { handleHTTPError } from '@shared-ui/utils/http.js'
import api from '@/api'
import { useEmitter } from '@main/composables/useEmitter'
import { EMITTER_EVENTS } from '@main/constants/emitterEvents.js'

const props = defineProps({
  inboxId: {
    type: [Number, String],
    required: true
  }
})

const emitter = useEmitter()
const opts = reactive({ conversation_limit: 50, message_limit: 50, resolve: true })
const job = ref(null)
const running = ref(false)
let timer = null

const poll = async () => {
  try {
    const resp = await api.getMetaHistoryImport(props.inboxId)
    job.value = resp.data.data
    running.value = !!job.value?.running
  } catch {
    // No import has run yet.
    running.value = false
  }
  if (!running.value) stopPolling()
}

const stopPolling = () => {
  if (timer) clearInterval(timer)
  timer = null
}

const start = async () => {
  try {
    await api.importMetaHistory(props.inboxId, {
      conversation_limit: Number(opts.conversation_limit) || 0,
      message_limit: Number(opts.message_limit) || 0,
      resolve: opts.resolve
    })
    running.value = true
    stopPolling()
    timer = setInterval(poll, 2000)
  } catch (error) {
    emitter.emit(EMITTER_EVENTS.SHOW_TOAST, {
      variant: 'destructive',
      description: handleHTTPError(error).message
    })
  }
}

onUnmounted(stopPolling)
poll()
</script>
