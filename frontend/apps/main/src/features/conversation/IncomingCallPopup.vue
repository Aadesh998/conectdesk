<template>
  <!-- Calls ringing this agent, so they know who it is before answering and
       can open the conversation. Agents who take calls in the browser answer
       there, on the softphone above. -->
  <div class="fixed bottom-4 right-4 z-50 flex w-80 flex-col gap-2">
    <SoftphoneCard />
    <div
      v-for="call in popups"
      :key="call.call_sid"
      class="rounded-lg border bg-background p-4 shadow-lg"
    >
      <div class="flex items-center gap-2">
        <PhoneIncoming :size="18" class="text-success animate-pulse" />
        <span class="font-semibold">{{ $t('voice.incomingCall') }}</span>
        <button type="button" class="ml-auto text-muted-foreground hover:text-foreground" @click="callStore.remove(call.call_sid)">✕</button>
      </div>
      <p class="mt-1 text-sm">{{ call.contact_name || call.from }}</p>
      <p class="text-xs text-muted-foreground">
        {{ call.from }} · {{ call.inbox_name }}<template v-if="call.menu"> · {{ call.menu }}</template>
      </p>
      <p class="mt-1 text-xs text-muted-foreground">
        {{ softphone.ringsBrowser ? $t('softphone.incomingHint') : $t('voice.incomingCall.hint') }}
      </p>
      <Button v-if="call.conversation_uuid" type="button" size="sm" class="mt-3 w-full" @click="open(call)">
        {{ $t('voice.openConversation') }}
      </Button>
    </div>
  </div>
</template>

<script setup>
import { computed } from 'vue'
import { useRouter } from 'vue-router'
import { PhoneIncoming } from 'lucide-vue-next'
import { Button } from '@shared-ui/components/ui/button'
import { useCallStore } from '@main/stores/calls'
import { useSoftphoneStore } from '@main/stores/softphone'
import SoftphoneCard from '@main/features/voice/SoftphoneCard.vue'

const router = useRouter()
const callStore = useCallStore()
const softphone = useSoftphoneStore()

// A call ringing in the browser shows on the softphone instead.
const popups = computed(() => callStore.ringing.filter((c) => !c.call_sid || c.call_sid !== softphone.call?.sid))

const open = (call) => {
  // The call's conversation is often unassigned yet, so open it under all.
  router.push({ name: 'inbox-conversation', params: { type: 'all', uuid: call.conversation_uuid } })
  callStore.remove(call.call_sid)
}
</script>
