<template>
  <!-- Click-to-call: Exotel rings the agent, on their phone or in this
       browser, then the customer.
       Shown whenever a phone inbox exists; disabled with the reason until the
       contact and the agent both have a number. -->
  <div v-if="voiceInboxes.length" class="flex items-center gap-1">
    <select
      v-if="voiceInboxes.length > 1 && !compact"
      v-model="inboxID"
      class="h-7 rounded-md border bg-background px-1 text-xs"
    >
      <option v-for="inb in voiceInboxes" :key="inb.id" :value="inb.id">{{ inb.name }}</option>
    </select>
    <Tooltip>
      <TooltipTrigger as-child>
        <!-- A wrapper, as a disabled button gets no hover for its tooltip. -->
        <span>
          <Button
            type="button"
            :variant="compact ? 'ghost' : 'outline'"
            size="sm"
            :class="compact ? 'h-8 w-8 p-0' : 'h-7 gap-1 px-2 text-xs'"
            :disabled="!!disabledReason"
            :is-loading="calling"
            :aria-label="$t('voice.call')"
            @click="call"
          >
            <Phone :size="compact ? 16 : 12" />
            <template v-if="!compact">{{ $t('voice.call') }}</template>
          </Button>
        </span>
      </TooltipTrigger>
      <TooltipContent>
        {{ disabledReason || $t('voice.callContact', { name: contactName }) }}
      </TooltipContent>
    </Tooltip>
  </div>
</template>

<script setup>
import { computed, onMounted, ref, watch } from 'vue'
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
  conversation: { type: Object, default: null },
  // Icon only, for the conversation header.
  compact: { type: Boolean, default: false }
})

const { t } = useI18n()
const emitter = useEmitter()
const inboxStore = useInboxStore()
const calling = ref(false)
const inboxID = ref(null)
const leg = useCallLeg()

const voiceInboxes = computed(() => inboxStore.inboxes.filter((i) => i.channel === 'voice' && i.enabled))
const hasPhone = computed(() => !!props.conversation?.contact?.phone_number)
const contactName = computed(() =>
  [props.conversation?.contact?.first_name, props.conversation?.contact?.last_name].filter(Boolean).join(' ')
)
const disabledReason = computed(() => {
  if (!hasPhone.value) return t('voice.callDisabled.noContactPhone')
  return leg.notReadyReason.value
})

// A phone conversation calls back on its own inbox; others use the first.
watch(
  [voiceInboxes, () => props.conversation?.inbox_id],
  () => {
    const own = voiceInboxes.value.find((i) => i.id === props.conversation?.inbox_id)
    inboxID.value = (own || voiceInboxes.value[0])?.id ?? null
  },
  { immediate: true }
)

onMounted(() => inboxStore.fetchInboxes())

const call = async () => {
  if (!props.conversation?.uuid || !inboxID.value) return
  calling.value = true
  try {
    leg.expectCall()
    const resp = await api.startCall(props.conversation.uuid, inboxID.value)
    leg.expectCall(leg.callSID(resp.data.data))
    emitter.emit(EMITTER_EVENTS.SHOW_TOAST, { description: leg.startedMessage.value })
  } catch (error) {
    emitter.emit(EMITTER_EVENTS.SHOW_TOAST, { variant: 'destructive', description: handleHTTPError(error).message })
  } finally {
    calling.value = false
  }
}
</script>
