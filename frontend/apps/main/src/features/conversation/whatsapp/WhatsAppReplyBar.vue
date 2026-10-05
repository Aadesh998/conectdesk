<template>
  <!-- WhatsApp takes free text only within 24 hours of the contact's last
       message; after that the agent has to send an approved template. -->
  <!-- A slim line while the window is open; a prominent one once it has closed. -->
  <div class="mx-2 mt-2 flex items-center justify-between gap-3 rounded-md border"
    :class="isOpen ? 'px-3 py-1 text-xs text-muted-foreground' : 'px-3 py-2 text-sm border-amber-500/50 bg-amber-500/10'">
    <span v-if="isOpen && replyWindow.closes_at">
      {{ $t('whatsapp.windowOpenUntil', { time: formatTime(replyWindow.closes_at) }) }}
    </span>
    <span v-else-if="!isOpen">{{ $t('whatsapp.windowClosedHint') }}</span>
    <span v-else />
    <Button type="button" size="sm" :variant="isOpen ? 'ghost' : 'default'" :class="{ 'h-7 text-xs': isOpen }" @click="open = true">
      {{ $t('whatsapp.sendTemplate') }}
    </Button>
  </div>

  <Dialog :open="open" @update:open="open = $event">
    <DialogContent class="max-w-lg">
      <DialogHeader>
        <DialogTitle>{{ $t('whatsapp.sendTemplate') }}</DialogTitle>
        <DialogDescription>{{ $t('whatsapp.sendTemplate.description') }}</DialogDescription>
      </DialogHeader>
      <WhatsAppTemplatePicker v-if="open" v-model="template" :inbox-id="inboxId" />
      <DialogFooter>
        <Button type="button" :disabled="!template || sending" :is-loading="sending" @click="send">
          {{ $t('globals.messages.send') }}
        </Button>
      </DialogFooter>
    </DialogContent>
  </Dialog>
</template>

<script setup>
import { computed, onUnmounted, ref, watch } from 'vue'
import { format } from 'date-fns'
import { Button } from '@shared-ui/components/ui/button'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle
} from '@shared-ui/components/ui/dialog'
import { handleHTTPError } from '@shared-ui/utils/http.js'
import api from '@main/api'
import { useEmitter } from '@main/composables/useEmitter'
import { EMITTER_EVENTS } from '@main/constants/emitterEvents.js'
import { UserTypeAgent } from '@/constants/user'
import WhatsAppTemplatePicker from './WhatsAppTemplatePicker.vue'

const props = defineProps({
  conversationUuid: {
    type: String,
    required: true
  },
  inboxId: {
    type: [Number, String],
    required: true
  },
  // Changes when a message arrives, which may reopen the window.
  lastMessageAt: {
    type: String,
    default: ''
  }
})

const emitter = useEmitter()
const replyWindow = ref({ open: true, closes_at: null })
// Ticks so the bar flips to "closed" when the window runs out on screen.
const now = ref(Date.now())
const ticker = setInterval(() => (now.value = Date.now()), 60 * 1000)
onUnmounted(() => clearInterval(ticker))
const isOpen = computed(
  () =>
    replyWindow.value.open &&
    (!replyWindow.value.closes_at || new Date(replyWindow.value.closes_at).getTime() > now.value)
)
const open = ref(false)
const template = ref(null)
const sending = ref(false)

const formatTime = (ts) => format(new Date(ts), 'MMM d, h:mm a')

const loadWindow = async () => {
  try {
    const resp = await api.getReplyWindow(props.conversationUuid)
    replyWindow.value = resp.data.data
  } catch (error) {
    // Not fatal: the server still refuses a reply the window doesn't allow.
    console.error('Error fetching reply window:', error)
  }
}

const send = async () => {
  sending.value = true
  try {
    await api.sendMessage(props.conversationUuid, {
      sender_type: UserTypeAgent,
      private: false,
      message: '',
      attachments: template.value.attachments,
      whatsapp_template: template.value
    })
    open.value = false
    template.value = null
  } catch (error) {
    emitter.emit(EMITTER_EVENTS.SHOW_TOAST, {
      variant: 'destructive',
      description: handleHTTPError(error).message
    })
  } finally {
    sending.value = false
  }
}

watch(() => [props.conversationUuid, props.lastMessageAt], loadWindow, { immediate: true })
</script>
