<template>
  <!-- Call any number: the caller ID is the phone inbox's ExoPhone, and
       Exotel rings the agent first, on their phone or in this browser, then
       the number typed. -->
  <Dialog v-model:open="open">
    <DialogContent class="sm:max-w-md">
      <DialogHeader>
        <DialogTitle>{{ $t('voice.dialer') }}</DialogTitle>
        <DialogDescription>{{ $t('voice.dialer.description') }}</DialogDescription>
      </DialogHeader>

      <form class="space-y-4" @submit.prevent="call">
        <div class="space-y-2">
          <Label for="dial-number">{{ $t('voice.dialer.to') }}</Label>
          <Input
            id="dial-number"
            ref="numberInput"
            v-model.trim="phone"
            type="tel"
            inputmode="tel"
            autocomplete="off"
            :placeholder="$t('voice.phoneNumber.placeholder')"
            class="text-lg tracking-wide"
          />
          <p class="text-xs text-muted-foreground">{{ $t('voice.dialer.toHint') }}</p>
        </div>

        <div v-if="!contactId" class="space-y-2">
          <Label for="dial-name">
            {{ $t('voice.dialer.name') }} ({{ $t('globals.terms.optional') }})
          </Label>
          <Input id="dial-name" v-model.trim="name" autocomplete="off" />
        </div>

        <div class="space-y-2">
          <Label>{{ $t('voice.dialer.from') }}</Label>
          <select
            v-if="voiceInboxes.length > 1"
            v-model="inboxID"
            class="h-9 w-full rounded-md border bg-background px-2 text-sm"
          >
            <option v-for="inb in voiceInboxes" :key="inb.id" :value="inb.id">
              {{ inb.name }} · {{ inb.config?.caller_id }}
            </option>
          </select>
          <p v-else-if="selectedInbox" class="rounded-md border bg-muted/40 px-3 py-2 text-sm">
            {{ selectedInbox.config?.caller_id || '—' }}
            <span class="text-muted-foreground">· {{ selectedInbox.name }}</span>
          </p>
          <p v-else class="text-sm text-destructive">{{ $t('voice.callDisabled.noInbox') }}</p>
        </div>

        <div class="rounded-md border px-3 py-2 text-sm">
          <template v-if="leg.inBrowser.value && !leg.notReadyReason.value">
            <span class="text-muted-foreground">{{ $t('voice.dialer.ringsFirst') }}</span>
            <span class="font-medium"> {{ $t('softphone.thisBrowser') }}</span>
          </template>
          <span v-else-if="leg.inBrowser.value" class="text-destructive">{{ leg.notReadyReason.value }}</span>
          <template v-else-if="userStore.phoneNumber">
            <span class="text-muted-foreground">{{ $t('voice.dialer.ringsFirst') }}</span>
            <span class="font-medium"> {{ userStore.phoneNumber }}</span>
          </template>
          <span v-else class="text-destructive">
            {{ $t('voice.newCallNoPhone') }}
            <router-link :to="{ name: 'profile' }" class="underline" @click="open = false">
              {{ $t('globals.terms.profile') }}
            </router-link>
          </span>
        </div>

        <DialogFooter>
          <Button type="button" variant="outline" @click="open = false">
            {{ $t('globals.messages.cancel') }}
          </Button>
          <Button type="submit" class="gap-1.5" :disabled="!canCall" :is-loading="calling">
            <Phone :size="14" />
            {{ $t('voice.startCall') }}
          </Button>
        </DialogFooter>
      </form>
    </DialogContent>
  </Dialog>
</template>

<script setup>
import { computed, nextTick, onMounted, onUnmounted, ref, watch } from 'vue'
import { useRouter } from 'vue-router'
import { Phone } from 'lucide-vue-next'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle
} from '@shared-ui/components/ui/dialog'
import { Button } from '@shared-ui/components/ui/button'
import { Input } from '@shared-ui/components/ui/input'
import { Label } from '@shared-ui/components/ui/label'
import { handleHTTPError } from '@shared-ui/utils/http.js'
import api from '@/api'
import { useInboxStore } from '@/stores/inbox'
import { useUserStore } from '@/stores/user'
import { useEmitter } from '@/composables/useEmitter'
import { useCallLeg } from '@/composables/useCallLeg'
import { EMITTER_EVENTS } from '@/constants/emitterEvents.js'

const router = useRouter()
const emitter = useEmitter()
const inboxStore = useInboxStore()
const userStore = useUserStore()

const open = ref(false)
const phone = ref('')
const name = ref('')
const contactId = ref(null)
const inboxID = ref(null)
const calling = ref(false)
const numberInput = ref(null)
const leg = useCallLeg()

const voiceInboxes = computed(() => inboxStore.inboxes.filter((i) => i.channel === 'voice' && i.enabled))
const selectedInbox = computed(() => voiceInboxes.value.find((i) => i.id === inboxID.value))
const digits = computed(() => phone.value.replace(/\D/g, ''))
const canCall = computed(
  () => digits.value.length >= 10 && !!selectedInbox.value && !leg.notReadyReason.value && !calling.value
)

watch(
  voiceInboxes,
  (list) => {
    if (!list.some((i) => i.id === inboxID.value)) inboxID.value = list[0]?.id ?? null
  },
  { immediate: true }
)

const show = (opts = {}) => {
  phone.value = opts.phone || ''
  name.value = opts.name || ''
  contactId.value = opts.contactId || null
  if (opts.inboxId) inboxID.value = opts.inboxId
  open.value = true
  inboxStore.fetchInboxes()
  nextTick(() => numberInput.value?.$el?.focus())
}

const call = async () => {
  if (!canCall.value) return
  calling.value = true
  try {
    leg.expectCall()
    const resp = await api.dial({
      phone: phone.value,
      name: name.value,
      inbox_id: inboxID.value,
      contact_id: contactId.value || 0
    })
    leg.expectCall(leg.callSID(resp.data.data))
    emitter.emit(EMITTER_EVENTS.SHOW_TOAST, { description: leg.startedMessage.value })
    open.value = false
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

onMounted(() => emitter.on(EMITTER_EVENTS.OPEN_DIALER, show))
onUnmounted(() => emitter.off(EMITTER_EVENTS.OPEN_DIALER, show))
</script>
