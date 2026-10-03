<template>
  <div class="box p-6 space-y-4 max-w-2xl">
    <div class="flex items-center gap-3">
      <component :is="channelIcon('telegram')" class="size-6" />
      <h3 class="font-semibold text-lg">{{ $t('globals.terms.telegram') }}</h3>
    </div>
    <p class="text-sm text-muted-foreground">{{ $t('telegram.help') }}</p>
    <p class="text-sm">{{ $t('telegram.steps') }}</p>

    <div class="space-y-1.5">
      <Label>{{ $t('telegram.token') }}</Label>
      <Input v-model="token" type="password" autocomplete="off" placeholder="123456789:AAE..." />
      <p class="text-xs text-muted-foreground">{{ $t('telegram.token.description') }}</p>
    </div>
    <div class="space-y-1.5">
      <Label>{{ $t('globals.terms.name') }} ({{ $t('globals.terms.optional') }})</Label>
      <Input v-model="name" />
    </div>
    <p class="text-xs text-muted-foreground">{{ $t('telegram.limits') }}</p>

    <Button type="button" :is-loading="connecting" :disabled="!token.trim() || connecting" @click="connect">
      {{ $t('telegram.connect') }}
    </Button>
  </div>
</template>

<script setup>
import { ref } from 'vue'
import { useRouter } from 'vue-router'
import { useI18n } from 'vue-i18n'
import { Button } from '@shared-ui/components/ui/button'
import { Input } from '@shared-ui/components/ui/input'
import { Label } from '@shared-ui/components/ui/label'
import { handleHTTPError } from '@shared-ui/utils/http.js'
import api from '@/api'
import { useEmitter } from '@main/composables/useEmitter'
import { EMITTER_EVENTS } from '@main/constants/emitterEvents.js'
import { channelIcon } from '@/features/conversation/channelIcon.js'

const { t } = useI18n()
const router = useRouter()
const emitter = useEmitter()
const token = ref('')
const name = ref('')
const connecting = ref(false)

const connect = async () => {
  connecting.value = true
  try {
    await api.connectTelegram({ bot_token: token.value.trim(), name: name.value.trim() })
    emitter.emit(EMITTER_EVENTS.SHOW_TOAST, { description: t('toast.inboxConnected') })
    router.push({ name: 'inbox-list' })
  } catch (error) {
    emitter.emit(EMITTER_EVENTS.SHOW_TOAST, { variant: 'destructive', description: handleHTTPError(error).message })
  } finally {
    connecting.value = false
  }
}
</script>
