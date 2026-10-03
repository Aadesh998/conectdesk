<template>
  <div class="h-full overflow-y-auto">
    <div class="p-4 md:p-6 w-full max-w-5xl space-y-4">
      <div class="flex flex-wrap items-center justify-between gap-3">
        <div>
          <h2 class="text-lg font-semibold">{{ $t('callBack.title') }}</h2>
          <p class="text-sm text-muted-foreground">{{ $t('callBack.description') }}</p>
        </div>
        <div class="flex flex-wrap items-center gap-2">
          <label class="flex items-center gap-2 text-sm text-muted-foreground">
            <input v-model="mineOnly" type="checkbox" class="size-4 accent-primary" />
            {{ $t('callBack.mineOnly') }}
          </label>
          <div class="flex rounded-md border p-0.5">
            <Button
              v-for="s in ['pending', 'done']"
              :key="s"
              size="sm"
              :variant="status === s ? 'default' : 'ghost'"
              class="h-7"
              @click="status = s"
            >
              {{ s === 'pending' ? $t('callBack.waiting', { n: store.counts.pending }) : $t('callBack.done') }}
            </Button>
          </div>
        </div>
      </div>

      <p v-if="!loading && !rows.length" class="box p-6 text-center text-sm text-muted-foreground">
        {{ status === 'pending' ? $t('callBack.noneWaiting') : $t('callBack.noneDone') }}
      </p>

      <div v-else class="space-y-2" :class="{ 'opacity-50 transition-opacity': loading }">
        <div
          v-for="cb in rows"
          :key="cb.id"
          class="box flex flex-wrap items-center gap-x-4 gap-y-2 p-3"
          :class="{ 'border-destructive/50': isOverdue(cb) }"
        >
          <PhoneMissed :size="18" class="shrink-0" :class="isOverdue(cb) ? 'text-destructive' : 'text-muted-foreground'" />

          <div class="min-w-0 flex-1">
            <div class="flex flex-wrap items-center gap-2">
              <span class="font-medium truncate">{{ contactName(cb) }}</span>
              <span v-if="contactName(cb) !== cb.phone" class="text-sm text-muted-foreground tabular-nums">{{ cb.phone }}</span>
              <span v-if="cb.missed_calls > 1" class="rounded bg-muted px-1.5 py-0.5 text-xs">
                {{ $t('callBack.missedTimes', { n: cb.missed_calls }, cb.missed_calls) }}
              </span>
              <span v-if="cb.voicemail" class="rounded bg-sky-500/15 px-1.5 py-0.5 text-xs text-sky-700 dark:text-sky-300">
                {{ $t('callBack.voicemail') }}
              </span>
              <span v-if="cb.attempts" class="rounded bg-muted px-1.5 py-0.5 text-xs">
                {{ $t('callBack.attempts', { n: cb.attempts }, cb.attempts) }}
              </span>
            </div>
            <p class="mt-0.5 text-xs text-muted-foreground">
              <template v-if="cb.status === 'pending'">
                <span :class="{ 'font-medium text-destructive': isOverdue(cb) }">{{ dueText(cb) }}</span>
                · {{ $t('callBack.missedAgo', { time: ago(cb.last_missed_at) }) }}
              </template>
              <template v-else>{{ doneText(cb) }}</template>
              · {{ cb.assignee_name || cb.team_name || $t('callBack.unassigned') }}
              · #{{ cb.reference_number }}
            </p>
            <p v-if="whatsAppText(cb)" class="mt-0.5 flex items-center gap-1 text-xs" :class="whatsAppClass(cb)">
              <MessageCircle :size="12" />
              <router-link
                v-if="cb.whatsapp_conversation_uuid"
                :to="{ name: 'inbox-conversation', params: { type: 'all', uuid: cb.whatsapp_conversation_uuid } }"
                class="hover:underline"
              >
                {{ whatsAppText(cb) }}
              </router-link>
              <span v-else>{{ whatsAppText(cb) }}</span>
            </p>
          </div>

          <div class="flex shrink-0 items-center gap-1.5">
            <Button
              v-if="cb.status === 'pending'"
              size="sm"
              class="h-8 gap-1"
              :disabled="!userStore.phoneNumber"
              :title="userStore.phoneNumber ? '' : $t('voice.callDisabled.noAgentPhone')"
              :is-loading="calling === cb.id"
              @click="callBack(cb)"
            >
              <Phone :size="14" /> {{ $t('callBack.callBack') }}
            </Button>
            <Button size="sm" variant="outline" class="h-8" @click="open(cb)">
              {{ $t('callBack.open') }}
            </Button>
            <Button
              v-if="cb.status === 'pending'"
              size="sm"
              variant="ghost"
              class="h-8"
              :is-loading="marking === cb.id"
              @click="markDone(cb)"
            >
              {{ $t('callBack.markDone') }}
            </Button>
          </div>
        </div>

        <div v-if="total > rows.length" class="pt-2 text-center">
          <Button variant="outline" size="sm" :is-loading="loading" @click="load(page + 1)">
            {{ $t('callBack.loadMore') }}
          </Button>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup>
import { onMounted, onUnmounted, ref, watch } from 'vue'
import { useRouter } from 'vue-router'
import { useI18n } from 'vue-i18n'
import { MessageCircle, Phone, PhoneMissed } from 'lucide-vue-next'
import { Button } from '@shared-ui/components/ui/button'
import { getRelativeTime } from '@shared-ui/utils/datetime.js'
import { handleHTTPError } from '@shared-ui/utils/http.js'
import api from '@main/api'
import { useCallBackStore } from '@main/stores/callBacks'
import { useUserStore } from '@main/stores/user'
import { useEmitter } from '@main/composables/useEmitter'
import { EMITTER_EVENTS } from '@main/constants/emitterEvents.js'

const PER_PAGE = 50

const { t } = useI18n()
const router = useRouter()
const emitter = useEmitter()
const store = useCallBackStore()
const userStore = useUserStore()

const status = ref('pending')
const mineOnly = ref(false)
const rows = ref([])
const total = ref(0)
const page = ref(1)
const loading = ref(false)
const calling = ref(null)
const marking = ref(null)
// Ticks each minute, so "due in" and "overdue" stay current.
const now = ref(Date.now())
let ticker = null

const toast = (description, variant) => emitter.emit(EMITTER_EVENTS.SHOW_TOAST, { description, variant })

const load = async (p = 1) => {
  loading.value = true
  try {
    const resp = await api.getCallBacks({ status: status.value, mine: mineOnly.value, page: p, per_page: PER_PAGE })
    const data = resp.data.data
    rows.value = p === 1 ? data.results || [] : [...rows.value, ...(data.results || [])]
    total.value = data.total || 0
    page.value = p
  } catch (error) {
    toast(handleHTTPError(error).message, 'destructive')
  } finally {
    loading.value = false
  }
}

const contactName = (cb) => [cb.contact_first_name, cb.contact_last_name].filter(Boolean).join(' ') || cb.phone
const isOverdue = (cb) => cb.status === 'pending' && new Date(cb.due_at).getTime() < now.value
const ago = (ts) => getRelativeTime(new Date(ts), new Date(now.value))

const dueText = (cb) => {
  const mins = Math.round((new Date(cb.due_at).getTime() - now.value) / 60000)
  if (mins < 0) return t('callBack.overdueBy', { time: ago(cb.due_at) })
  return mins === 0 ? t('callBack.dueNow') : t('callBack.dueIn', { n: mins })
}

const doneText = (cb) => {
  const key = { called_back: 'callBack.doneCalledBack', answered: 'callBack.doneAnswered', manual: 'callBack.doneManual' }[cb.done_reason]
  return t(key || 'callBack.done') + ' · ' + ago(cb.done_at)
}

const whatsAppText = (cb) => {
  if (cb.whatsapp_error) return t('callBack.whatsappFailed')
  if (!cb.whatsapp_status) return ''
  return t(`callBack.whatsapp.${cb.whatsapp_status}`)
}
const whatsAppClass = (cb) =>
  cb.whatsapp_error || cb.whatsapp_status === 'failed' ? 'text-destructive' : 'text-emerald-700 dark:text-emerald-400'

// Calls the contact from their phone conversation's line: the agent's phone
// rings first. The call-back closes by itself once the call connects.
const callBack = async (cb) => {
  calling.value = cb.id
  try {
    await api.startCall(cb.conversation_uuid, cb.inbox_id)
    toast(t('voice.callStarted'))
  } catch (error) {
    toast(handleHTTPError(error).message, 'destructive')
  } finally {
    calling.value = null
  }
}

const open = (cb) => router.push({ name: 'inbox-conversation', params: { type: 'all', uuid: cb.conversation_uuid } })

const markDone = async (cb) => {
  marking.value = cb.id
  try {
    await api.markCallBackDone(cb.id)
    rows.value = rows.value.filter((r) => r.id !== cb.id)
    total.value = Math.max(0, total.value - 1)
    store.fetchCounts()
  } catch (error) {
    toast(handleHTTPError(error).message, 'destructive')
  } finally {
    marking.value = null
  }
}

watch([status, mineOnly], () => load(1))
watch(() => store.version, () => load(1))

onMounted(() => {
  load(1)
  store.fetchCounts()
  ticker = setInterval(() => (now.value = Date.now()), 60 * 1000)
})
onUnmounted(() => clearInterval(ticker))
</script>
