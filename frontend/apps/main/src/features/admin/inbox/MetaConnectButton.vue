<template>
  <Button
    type="button"
    :variant="variant"
    :is-loading="connecting"
    :disabled="!available || connecting"
    @click="connect"
  >
    {{ label || $t('admin.inbox.meta.connect') }}
  </Button>
</template>

<script setup>
import { computed, ref } from 'vue'
import { Button } from '@shared-ui/components/ui/button/index.js'
import { handleHTTPError } from '@shared-ui/utils/http.js'
import api from '@/api'
import { useAppSettingsStore } from '@/stores/appSettings'
import { useEmitter } from '@main/composables/useEmitter'
import { EMITTER_EVENTS } from '@main/constants/emitterEvents.js'

const props = defineProps({
  // whatsapp, messenger or instagram.
  channel: {
    type: String,
    required: true
  },
  label: {
    type: String,
    default: ''
  },
  variant: {
    type: String,
    default: 'default'
  }
})

const emitter = useEmitter()
const appSettingsStore = useAppSettingsStore()
const connecting = ref(false)

// The channels the platform's Meta app is set up for.
const available = computed(() =>
  (appSettingsStore.public_config['app.meta_products'] || []).includes(props.channel)
)

// Facebook Login runs as a full-page redirect, not the JavaScript SDK's popup:
// the SDK only works on domains listed in the Meta app, and every workspace
// has its own. The login returns through the shared redirect URL on the base
// domain, which sends the browser back to Inboxes here.
const connect = async () => {
  connecting.value = true
  try {
    const response = await api.metaAuthorize(props.channel)
    window.location.href = response.data.data
  } catch (error) {
    connecting.value = false
    emitter.emit(EMITTER_EVENTS.SHOW_TOAST, {
      variant: 'destructive',
      description: handleHTTPError(error).message
    })
  }
}
</script>
