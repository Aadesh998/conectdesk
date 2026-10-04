<template>
  <!-- Calls a contact from a phone inbox: Exotel rings the agent first, on
       their phone or in this browser. -->
  <Tooltip v-if="voiceInboxes.length">
    <TooltipTrigger as-child>
      <span @click.stop>
        <Button
          type="button"
          variant="outline"
          size="sm"
          class="h-8 gap-1.5"
          :disabled="!!disabledReason"
          :is-loading="calling"
          @click.stop="call"
        >
          <Phone :size="14" />
          {{ $t('voice.call') }}
        </Button>
      </span>
    </TooltipTrigger>
    <TooltipContent>{{ disabledReason || $t('voice.callContact', { name: fullName }) }}</TooltipContent>
  </Tooltip>
</template>

<script setup>
import { computed, ref } from 'vue'
import { useRouter } from 'vue-router'
import { useI18n } from 'vue-i18n'
import { Phone } from 'lucide-vue-next'
import { Button } from '@shared-ui/components/ui/button'
import { Tooltip, TooltipContent, TooltipTrigger } from '@shared-ui/components/ui/tooltip'
import { handleHTTPError } from '@shared-ui/utils/http.js'
import api from '@/api'
import { useInboxStore } from '@/stores/inbox'
import { useEmitter } from '@/composables/useEmitter'
import { useCallLeg } from '@/composables/useCallLeg'
import { EMITTER_EVENTS } from '@/constants/emitterEvents.js'

const props = defineProps({
  contact: { type: Object, required: true }
})

const { t } = useI18n()
const router = useRouter()
const emitter = useEmitter()
const inboxStore = useInboxStore()
const calling = ref(false)
const leg = useCallLeg()

const voiceInboxes = computed(() => inboxStore.inboxes.filter((i) => i.channel === 'voice' && i.enabled))
const fullName = computed(() =>
  [props.contact.first_name, props.contact.last_name].filter(Boolean).join(' ')
)
const disabledReason = computed(() => {
  if (!props.contact.phone_number) return t('voice.callDisabled.noContactPhone')
  return leg.notReadyReason.value
})

const call = async () => {
  if (disabledReason.value) return
  // With several phone inboxes, the dialer asks which number to call from.
  if (voiceInboxes.value.length > 1) {
    emitter.emit(EMITTER_EVENTS.OPEN_DIALER, {
      phone: props.contact.phone_number,
      name: fullName.value,
      contactId: props.contact.id
    })
    return
  }
  calling.value = true
  try {
    leg.expectCall()
    const resp = await api.callContact(props.contact.id, voiceInboxes.value[0].id)
    leg.expectCall(leg.callSID(resp.data.data))
    emitter.emit(EMITTER_EVENTS.SHOW_TOAST, { description: leg.startedMessage.value })
    const uuid = resp.data.data?.conversation_uuid
    if (uuid) router.push({ name: 'inbox-conversation', params: { type: 'all', uuid } })
  } catch (error) {
    emitter.emit(EMITTER_EVENTS.SHOW_TOAST, {
      variant: 'destructive',
      description: handleHTTPError(error).message
    })
  } finally {
    calling.value = false
  }
}
</script>
