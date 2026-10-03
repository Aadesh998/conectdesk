<template>
  <div class="space-y-2 text-sm">
    <div v-if="status" class="flex items-center gap-2">
      <span class="inline-block size-2 rounded-full" :class="status.connected ? 'bg-green-500' : 'bg-destructive'" />
      {{ status.connected ? $t('telegram.webhook.ok') : $t('telegram.webhook.notConnected') }}
    </div>
    <p v-if="status?.pending_update_count" class="text-xs text-muted-foreground">
      {{ $t('telegram.webhook.pending', { count: status.pending_update_count }) }}
    </p>
    <p v-if="status?.last_error_message" class="text-xs text-destructive">
      {{ $t('telegram.webhook.lastError', { error: status.last_error_message }) }}
    </p>
    <Button type="button" variant="outline" size="sm" :is-loading="saving" @click="reconnect">
      {{ $t('telegram.webhook.reconnect') }}
    </Button>
  </div>
</template>

<script setup>
import { ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { Button } from '@shared-ui/components/ui/button'
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

const { t } = useI18n()
const emitter = useEmitter()
const status = ref(null)
const saving = ref(false)

const load = async () => {
  try {
    const resp = await api.getTelegramWebhook(props.inboxId)
    status.value = resp.data.data
  } catch (error) {
    emitter.emit(EMITTER_EVENTS.SHOW_TOAST, { variant: 'destructive', description: handleHTTPError(error).message })
  }
}

const reconnect = async () => {
  saving.value = true
  try {
    await api.setTelegramWebhook(props.inboxId)
    emitter.emit(EMITTER_EVENTS.SHOW_TOAST, { description: t('telegram.webhook.reconnected') })
    await load()
  } catch (error) {
    emitter.emit(EMITTER_EVENTS.SHOW_TOAST, { variant: 'destructive', description: handleHTTPError(error).message })
  } finally {
    saving.value = false
  }
}

watch(() => props.inboxId, load, { immediate: true })
</script>
