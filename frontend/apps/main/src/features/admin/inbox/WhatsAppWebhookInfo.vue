<template>
  <div class="space-y-2">
    <p class="text-sm font-medium">{{ $t('whatsapp.manual.webhook') }}</p>
    <p class="text-xs text-muted-foreground">{{ $t('whatsapp.manual.webhook.description') }}</p>
    <div v-for="field in fields" :key="field.label" class="space-y-1">
      <Label class="text-xs">{{ field.label }}</Label>
      <div class="flex gap-2">
        <Input :model-value="field.value" readonly class="font-mono text-xs" />
        <Button type="button" variant="outline" size="sm" @click="copy(field.value)">
          {{ $t('whatsapp.manual.copy') }}
        </Button>
      </div>
    </div>
    <p class="text-xs text-muted-foreground">{{ $t('whatsapp.manual.webhook.fields') }}</p>
  </div>
</template>

<script setup>
import { computed, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { Button } from '@shared-ui/components/ui/button'
import { Input } from '@shared-ui/components/ui/input'
import { Label } from '@shared-ui/components/ui/label'
import { handleHTTPError } from '@shared-ui/utils/http.js'
import api from '@/api'
import { useEmitter } from '@main/composables/useEmitter'
import { EMITTER_EVENTS } from '@main/constants/emitterEvents.js'

const props = defineProps({
  // Fetch the inbox's webhook details, or show the ones given.
  inboxId: { type: Number, default: null },
  webhookUrl: { type: String, default: '' },
  verifyToken: { type: String, default: '' }
})

const { t } = useI18n()
const emitter = useEmitter()
const url = ref(props.webhookUrl)
const token = ref(props.verifyToken)

const fields = computed(() => [
  { label: t('whatsapp.manual.webhook.url'), value: url.value },
  { label: t('whatsapp.manual.webhook.verifyToken'), value: token.value }
])

onMounted(async () => {
  if (!props.inboxId) return
  try {
    const resp = await api.getWhatsAppWebhook(props.inboxId)
    url.value = resp.data.data.webhook_url
    token.value = resp.data.data.verify_token
  } catch (error) {
    emitter.emit(EMITTER_EVENTS.SHOW_TOAST, { variant: 'destructive', description: handleHTTPError(error).message })
  }
})

const copy = async (value) => {
  try {
    await navigator.clipboard.writeText(value)
    emitter.emit(EMITTER_EVENTS.SHOW_TOAST, { description: t('globals.messages.copied') })
  } catch {
    // Clipboard blocked; the field is selectable.
  }
}
</script>
