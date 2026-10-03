<template>
  <!-- Hands the customer to another agent: the conversation is reassigned and
       Exotel calls the customer from that agent's phone. -->
  <div v-if="agents.length" class="flex items-center gap-1">
    <select v-model="agentID" class="h-7 rounded-md border bg-background px-1 text-xs">
      <option :value="null" disabled>{{ $t('voice.transferTo') }}</option>
      <option v-for="a in agents" :key="a.id" :value="a.id">
        {{ a.name }}{{ a.availability === 'online' ? '' : ` (${a.availability})` }}
      </option>
    </select>
    <Button type="button" variant="outline" size="sm" class="h-7 px-2 text-xs" :disabled="!agentID" :is-loading="busy" @click="transfer">
      {{ $t('voice.transfer') }}
    </Button>
  </div>
</template>

<script setup>
import { computed, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { Button } from '@shared-ui/components/ui/button'
import { handleHTTPError } from '@shared-ui/utils/http.js'
import api from '@/api'
import { useInboxStore } from '@/stores/inbox'
import { useUserStore } from '@/stores/user'
import { useEmitter } from '@/composables/useEmitter'
import { EMITTER_EVENTS } from '@/constants/emitterEvents.js'

const props = defineProps({
  conversation: { type: Object, default: null }
})

const { t } = useI18n()
const emitter = useEmitter()
const inboxStore = useInboxStore()
const userStore = useUserStore()
const all = ref([])
const agentID = ref(null)
const busy = ref(false)

const agents = computed(() => all.value.filter((a) => a.id !== userStore.userID))
const inboxID = computed(() => {
  const voice = inboxStore.inboxes.filter((i) => i.channel === 'voice' && i.enabled)
  return (voice.find((i) => i.id === props.conversation?.inbox_id) || voice[0])?.id
})

onMounted(async () => {
  try {
    const resp = await api.getPhoneAgents()
    all.value = resp.data.data || []
  } catch {
    all.value = []
  }
})

const transfer = async () => {
  if (!props.conversation?.uuid || !agentID.value || !inboxID.value) return
  busy.value = true
  try {
    await api.transferCall(props.conversation.uuid, agentID.value, inboxID.value)
    emitter.emit(EMITTER_EVENTS.SHOW_TOAST, { description: t('voice.transferred') })
    agentID.value = null
  } catch (error) {
    emitter.emit(EMITTER_EVENTS.SHOW_TOAST, { variant: 'destructive', description: handleHTTPError(error).message })
  } finally {
    busy.value = false
  }
}
</script>
