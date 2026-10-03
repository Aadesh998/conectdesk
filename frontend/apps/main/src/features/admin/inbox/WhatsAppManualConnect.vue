<template>
  <div class="space-y-4">
    <template v-if="!result">
      <div>
        <h4 class="font-semibold">{{ reconnect ? $t('whatsapp.manual.updateTitle') : $t('whatsapp.manual.title') }}</h4>
        <p class="text-sm text-muted-foreground">{{ $t('whatsapp.manual.help') }}</p>
      </div>
      <p class="text-xs text-muted-foreground">{{ $t('whatsapp.manual.steps') }}</p>

      <div class="grid gap-4 sm:grid-cols-2">
        <div v-if="!reconnect" class="space-y-1.5 sm:col-span-2">
          <Label>{{ $t('globals.terms.name') }} ({{ $t('globals.terms.optional') }})</Label>
          <Input v-model="form.name" autocomplete="off" />
        </div>
        <div class="space-y-1.5">
          <Label>{{ $t('whatsapp.manual.wabaId') }}</Label>
          <Input v-model="form.waba_id" inputmode="numeric" autocomplete="off" />
        </div>
        <div class="space-y-1.5">
          <Label>{{ $t('whatsapp.manual.phoneNumberId') }}</Label>
          <Input v-model="form.phone_number_id" inputmode="numeric" autocomplete="off" />
        </div>
        <div class="space-y-1.5">
          <Label>{{ $t('whatsapp.manual.appId') }}</Label>
          <Input v-model="form.app_id" inputmode="numeric" autocomplete="off" />
        </div>
        <div class="space-y-1.5">
          <Label>{{ $t('whatsapp.manual.appSecret') }}</Label>
          <Input v-model="form.app_secret" type="password" autocomplete="new-password" />
        </div>
        <div class="space-y-1.5 sm:col-span-2">
          <Label>{{ $t('whatsapp.manual.accessToken') }}</Label>
          <Input v-model="form.access_token" type="password" autocomplete="new-password" />
          <p class="text-xs text-muted-foreground">{{ $t('whatsapp.manual.accessToken.description') }}</p>
        </div>
        <div class="space-y-1.5 sm:col-span-2">
          <Label>{{ $t('whatsapp.manual.pin') }} ({{ $t('globals.terms.optional') }})</Label>
          <Input v-model="form.two_step_pin" type="password" inputmode="numeric" maxlength="6" autocomplete="new-password" />
          <p class="text-xs text-muted-foreground">{{ $t('whatsapp.manual.pin.description') }}</p>
        </div>
      </div>

      <p class="text-xs text-muted-foreground flex items-center gap-1.5">
        <Lock class="size-3.5 shrink-0" />
        {{ $t('whatsapp.manual.encrypted') }}
      </p>
      <p v-if="!supported" class="text-sm text-destructive">{{ $t('sealed.insecureContext') }}</p>

      <Button type="button" :is-loading="connecting" :disabled="!complete || connecting || !supported" @click="connect">
        {{ reconnect ? $t('whatsapp.manual.update') : $t('whatsapp.manual.connect') }}
      </Button>
    </template>

    <!-- Meta couldn't be pointed at the inbox's webhook automatically: the admin
         enters it in their app instead. -->
    <template v-else>
      <p class="text-sm font-medium">{{ $t('toast.inboxConnected') }}</p>
      <p class="text-sm text-destructive">{{ $t('whatsapp.manual.webhookFailed', { error: result.webhook_error }) }}</p>
      <p v-if="result.registration_error" class="text-sm text-destructive">
        {{ $t('admin.inbox.meta.registrationError', { error: result.registration_error }) }}
      </p>
      <WhatsAppWebhookInfo :webhook-url="result.webhook_url" :verify-token="result.verify_token" />
      <Button type="button" @click="done">{{ $t('whatsapp.manual.done') }}</Button>
    </template>
  </div>
</template>

<script setup>
import { computed, reactive, ref } from 'vue'
import { useRouter } from 'vue-router'
import { useI18n } from 'vue-i18n'
import { Lock } from 'lucide-vue-next'
import { Button } from '@shared-ui/components/ui/button'
import { Input } from '@shared-ui/components/ui/input'
import { Label } from '@shared-ui/components/ui/label'
import { handleHTTPError } from '@shared-ui/utils/http.js'
import api from '@/api'
import { useEmitter } from '@main/composables/useEmitter'
import { EMITTER_EVENTS } from '@main/constants/emitterEvents.js'
import { sealPayload, sealingSupported } from '@/utils/sealed.js'
import WhatsAppWebhookInfo from './WhatsAppWebhookInfo.vue'

// Must match sealedPurposeWhatsAppManual on the server.
const PURPOSE = 'whatsapp_manual'

const props = defineProps({
  // Renewing an existing inbox's credentials, from its edit page.
  reconnect: { type: Boolean, default: false }
})
const emit = defineEmits(['connected'])

const { t } = useI18n()
const router = useRouter()
const emitter = useEmitter()
const connecting = ref(false)
const result = ref(null)
const supported = sealingSupported()

const empty = () => ({
  name: '',
  waba_id: '',
  phone_number_id: '',
  app_id: '',
  app_secret: '',
  access_token: '',
  two_step_pin: ''
})
const form = reactive(empty())

const complete = computed(
  () =>
    form.waba_id.trim() &&
    form.phone_number_id.trim() &&
    form.app_id.trim() &&
    form.app_secret.trim() &&
    form.access_token.trim()
)

const toastError = (description) =>
  emitter.emit(EMITTER_EVENTS.SHOW_TOAST, { variant: 'destructive', description })

const connect = async () => {
  connecting.value = true
  try {
    // A fresh key per attempt: the server deletes each one as it uses it.
    const keyResp = await api.getWhatsAppManualKey()
    const envelope = await sealPayload(keyResp.data.data, PURPOSE, { ...form })
    const resp = await api.connectWhatsAppManual(envelope)
    // Don't keep the secrets in the page once they've been sent.
    Object.assign(form, empty())

    const data = resp.data.data
    if (data.webhook_subscribed) {
      emitter.emit(EMITTER_EVENTS.SHOW_TOAST, { description: t('toast.inboxConnected') })
      done(data)
    } else {
      result.value = data
    }
  } catch (error) {
    toastError(error?.message === 'secure-context-required' ? t('sealed.insecureContext') : handleHTTPError(error).message)
  } finally {
    connecting.value = false
  }
}

const done = (data = result.value) => {
  if (props.reconnect) {
    result.value = null
    emit('connected', data)
    return
  }
  router.push({ name: 'inbox-list' })
}
</script>
