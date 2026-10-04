<template>
  <!-- The call in this browser, for agents who take calls here: answer or
       decline it, then mute, hold, dial keys and hang up. -->
  <div v-if="softphone.call" class="rounded-lg border bg-background p-4 shadow-lg" role="region" :aria-label="$t('softphone.title')">
    <div class="flex items-center gap-2">
      <component
        :is="softphone.call.incoming ? PhoneIncoming : PhoneOutgoing"
        :size="18"
        class="text-success"
        :class="{ 'animate-pulse': softphone.call.state === 'ringing' }"
      />
      <span class="font-semibold">{{ heading }}</span>
      <span v-if="softphone.call.state === 'connected'" class="ml-auto font-mono text-sm tabular-nums text-muted-foreground">
        {{ elapsed }}
      </span>
    </div>
    <p v-if="caller.name" class="mt-1 text-sm">{{ caller.name }}</p>
    <p v-if="caller.detail" class="text-xs text-muted-foreground">{{ caller.detail }}</p>

    <!-- Ringing in: answer or decline. -->
    <div v-if="softphone.call.state === 'ringing' && softphone.call.incoming" class="mt-3 grid grid-cols-2 gap-2">
      <Button type="button" variant="outline" size="sm" class="gap-1.5" @click="softphone.hangup()">
        <PhoneOff :size="14" />
        {{ $t('softphone.decline') }}
      </Button>
      <Button type="button" size="sm" class="gap-1.5 bg-success text-success-foreground hover:bg-success/90" @click="softphone.answer()">
        <Phone :size="14" />
        {{ $t('softphone.answer') }}
      </Button>
    </div>

    <template v-else>
      <div class="mt-3 flex items-center gap-2">
        <Button
          type="button"
          variant="outline"
          size="sm"
          class="h-9 w-9 p-0"
          :aria-pressed="softphone.call.muted"
          :aria-label="softphone.call.muted ? $t('softphone.unmute') : $t('softphone.mute')"
          :disabled="softphone.call.state !== 'connected'"
          @click="softphone.toggleMute()"
        >
          <MicOff v-if="softphone.call.muted" :size="16" class="text-destructive" />
          <Mic v-else :size="16" />
        </Button>
        <Button
          type="button"
          variant="outline"
          size="sm"
          class="h-9 w-9 p-0"
          :aria-pressed="softphone.call.held"
          :aria-label="softphone.call.held ? $t('softphone.resume') : $t('softphone.hold')"
          :disabled="softphone.call.state !== 'connected'"
          @click="softphone.toggleHold()"
        >
          <Play v-if="softphone.call.held" :size="16" />
          <Pause v-else :size="16" />
        </Button>
        <Button
          type="button"
          variant="outline"
          size="sm"
          class="h-9 w-9 p-0"
          :aria-pressed="showKeypad"
          :aria-label="$t('softphone.keypad')"
          :disabled="softphone.call.state !== 'connected'"
          @click="showKeypad = !showKeypad"
        >
          <Grid3x3 :size="16" />
        </Button>
        <Button type="button" variant="destructive" size="sm" class="ml-auto gap-1.5" @click="softphone.hangup()">
          <PhoneOff :size="14" />
          {{ $t('softphone.hangUp') }}
        </Button>
      </div>
      <div v-if="showKeypad && softphone.call.state === 'connected'" class="mt-3 grid grid-cols-3 gap-1.5">
        <Button
          v-for="d in keys"
          :key="d"
          type="button"
          variant="outline"
          size="sm"
          class="h-9 font-mono text-base"
          @click="softphone.sendDigit(d)"
        >
          {{ d }}
        </Button>
      </div>
    </template>
  </div>

  <!-- Browser calling is on but this tab can't take calls. -->
  <div
    v-else-if="problem"
    class="flex items-start gap-2 rounded-lg border border-destructive/40 bg-background p-3 text-sm shadow-lg"
  >
    <AlertTriangle :size="16" class="mt-0.5 shrink-0 text-destructive" />
    <div class="min-w-0 flex-1">
      <p class="font-medium">{{ $t('softphone.offline') }}</p>
      <p class="text-xs text-muted-foreground">{{ problem }}</p>
    </div>
    <Button type="button" variant="outline" size="sm" class="h-7 px-2 text-xs" @click="retry">
      {{ $t('softphone.retry') }}
    </Button>
  </div>
</template>

<script setup>
import { computed, onUnmounted, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { AlertTriangle, Grid3x3, Mic, MicOff, Pause, Phone, PhoneIncoming, PhoneOff, PhoneOutgoing, Play } from 'lucide-vue-next'
import ringtoneURL from '@exotel-npm-dev/webrtc-client-sdk/dist/ringtone.wav'
import { Button } from '@shared-ui/components/ui/button'
import { useSoftphoneStore } from '@main/stores/softphone'
import { useCallStore } from '@main/stores/calls'

const { t } = useI18n()
const softphone = useSoftphoneStore()
const callStore = useCallStore()
const showKeypad = ref(false)
const now = ref(Date.now())
const keys = ['1', '2', '3', '4', '5', '6', '7', '8', '9', '*', '0', '#']

// Who's calling, from the pop-up the server sent for the same call.
const caller = computed(() => {
  const c = softphone.call
  const known = callStore.ringing.find((r) => r.call_sid && r.call_sid === c?.sid)
  if (known) {
    return {
      name: known.contact_name || known.from,
      detail: [known.from, known.inbox_name, known.menu].filter(Boolean).join(' · ')
    }
  }
  return { name: c?.incoming ? c.from : '', detail: c?.incoming ? '' : t('softphone.connectingCustomer') }
})

const heading = computed(() => {
  const c = softphone.call
  if (c.state === 'connected') return c.held ? t('softphone.onHold') : t('softphone.inCall')
  return c.incoming ? t('voice.incomingCall') : t('softphone.calling')
})

const elapsed = computed(() => {
  const s = Math.max(0, Math.floor((now.value - (softphone.call?.connectedAt || now.value)) / 1000))
  const mm = String(Math.floor(s / 60) % 60).padStart(2, '0')
  const ss = String(s % 60).padStart(2, '0')
  return s >= 3600 ? `${Math.floor(s / 3600)}:${mm}:${ss}` : `${mm}:${ss}`
})

const problem = computed(() => {
  if (softphone.status === 'mic_blocked') return t('softphone.micBlocked')
  if (softphone.status === 'failed') return t('softphone.failed')
  return ''
})

const retry = () => {
  softphone.stop()
  softphone.start()
}

// A ring for incoming calls, and a clock for the one in progress.
const ring = new Audio(ringtoneURL)
ring.loop = true
let timer = null
watch(
  () => [softphone.call?.state, softphone.call?.incoming],
  ([state, incoming]) => {
    if (state === 'ringing' && incoming) ring.play().catch(() => {})
    else {
      ring.pause()
      ring.currentTime = 0
    }
    clearInterval(timer)
    timer = null
    if (state === 'connected') timer = setInterval(() => (now.value = Date.now()), 1000)
    if (!state) showKeypad.value = false
  }
)
onUnmounted(() => {
  ring.pause()
  clearInterval(timer)
})
</script>
