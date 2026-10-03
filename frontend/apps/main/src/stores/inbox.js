import { ref, computed } from 'vue'
import { defineStore } from 'pinia'
import { handleHTTPError } from '@shared-ui/utils/http.js'
import { useEmitter } from '../composables/useEmitter'
import { EMITTER_EVENTS } from '../constants/emitterEvents'
import api from '../api'

export const useInboxStore = defineStore('inbox', () => {
  const inboxes = ref([])
  const emitter = useEmitter()
  const options = computed(() => inboxes.value.map(inb => ({
    label: inb.name,
    value: String(inb.id)
  })))
  const emailOptions = computed(() => inboxes.value
    .filter(inb => inb.channel === 'email')
    .map(inb => ({ label: inb.name, value: String(inb.id) }))
  )
  // Inboxes an agent can start a conversation from: email, and WhatsApp, which
  // opens with an approved template sent to a phone number. Live chat is absent
  // because only a visitor can open one, and Messenger and Instagram only allow
  // a business to message someone who wrote first.
  const outboundOptions = computed(() => inboxes.value
    .filter(inb => ['email', 'whatsapp', 'voice'].includes(inb.channel) && inb.enabled !== false)
    .map(inb => ({ label: inb.name, value: String(inb.id), channel: inb.channel }))
  )
  const fetchInboxes = async (force = false) => {
    if (!force && inboxes.value.length) return
    try {
      const response = await api.getInboxes()
      inboxes.value = response?.data?.data || []
    } catch (error) {
      emitter.emit(EMITTER_EVENTS.SHOW_TOAST, {
        variant: 'destructive',
        description: handleHTTPError(error).message
      })
    }
  }
  return {
    inboxes,
    options,
    emailOptions,
    outboundOptions,
    fetchInboxes,
  }
})