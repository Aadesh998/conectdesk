<template>
  <div class="flex h-full flex-col overflow-hidden">
    <div class="flex flex-wrap items-center justify-between gap-3 border-b p-4">
      <div>
        <h2 class="text-lg font-semibold">{{ $t('lead.title') }}</h2>
        <p class="text-sm text-muted-foreground">{{ $t('lead.description') }}</p>
      </div>
      <div class="flex items-center gap-2">
        <Input v-model="search" class="h-8 w-56" :placeholder="$t('lead.search')" />
        <router-link v-if="userStore.can('general_settings:manage')" :to="{ name: 'lead-settings' }">
          <Button variant="outline" size="sm" class="h-8">{{ $t('lead.editStages') }}</Button>
        </router-link>
      </div>
    </div>

    <div class="flex flex-1 gap-3 overflow-x-auto p-4" :class="{ 'opacity-60 transition-opacity': loading }">
      <div
        v-for="col in columns"
        :key="col.stage.key"
        class="flex w-72 shrink-0 flex-col rounded-lg bg-muted/40"
        :class="{ 'ring-2 ring-primary': dropTarget === col.stage.key }"
        @dragover.prevent="dropTarget = col.stage.key"
        @dragleave="dropTarget = dropTarget === col.stage.key ? null : dropTarget"
        @drop.prevent="drop(col.stage.key)"
      >
        <div class="flex items-center justify-between px-3 py-2">
          <span class="flex items-center gap-2 text-sm font-semibold">
            <span class="size-2 rounded-full" :class="kindDot(col.stage.kind)" />
            {{ col.stage.label }}
          </span>
          <span class="text-xs text-muted-foreground tabular-nums">{{ col.total }}</span>
        </div>

        <div class="flex-1 space-y-2 overflow-y-auto px-2 pb-2">
          <div
            v-for="card in col.cards"
            :key="card.id"
            draggable="true"
            role="link"
            tabindex="0"
            class="box cursor-pointer space-y-1.5 p-2.5 text-sm transition-colors hover:border-primary/50 active:cursor-grabbing"
            @dragstart="dragging = { id: card.id, from: col.stage.key }"
            @dragend="dragging = null; dropTarget = null"
            @click="open(card)"
            @keydown.enter="open(card)"
          >
            <div class="flex items-start justify-between gap-2">
              <router-link :to="{ name: 'contact-detail', params: { id: card.id } }" class="font-medium hover:underline" @click.stop>
                {{ [card.first_name, card.last_name].filter(Boolean).join(' ') || card.phone_number }}
              </router-link>
              <div class="flex shrink-0 items-center gap-1">
                <span
                  v-if="card.pending_call_back"
                  class="rounded bg-destructive/15 px-1.5 py-0.5 text-[11px] text-destructive"
                  :title="$t('lead.callBackWaiting')"
                >
                  <PhoneMissed :size="12" class="inline" />
                </span>
                <button
                  v-if="userStore.hasAdminRole"
                  type="button"
                  class="rounded p-0.5 text-muted-foreground hover:bg-destructive/15 hover:text-destructive"
                  :title="$t('lead.delete')"
                  :aria-label="$t('lead.delete')"
                  @click.stop="toDelete = card"
                  @keydown.enter.stop
                >
                  <Trash2 :size="14" />
                </button>
              </div>
            </div>
            <p v-if="card.phone_number && card.phone_number !== card.first_name" class="text-xs text-muted-foreground tabular-nums">
              {{ card.phone_number }}
            </p>
            <div v-if="details(card).length" class="flex flex-wrap gap-1">
              <span v-for="d in details(card)" :key="d" class="rounded bg-muted px-1.5 py-0.5 text-[11px]">{{ d }}</span>
            </div>
            <div class="flex items-center justify-between gap-2 pt-0.5">
              <router-link
                v-if="card.last_conversation_uuid"
                :to="conversationRoute(card)"
                class="text-xs text-muted-foreground hover:text-foreground hover:underline"
                @click.stop
              >
                {{ $t('lead.lastActivity', { time: ago(card.last_message_at || card.stage_changed_at) }) }}
              </router-link>
              <span v-else class="text-xs text-muted-foreground">{{ ago(card.stage_changed_at) }}</span>
              <!-- Moving without dragging, for touch screens. -->
              <select
                :value="col.stage.key"
                class="h-6 max-w-28 rounded border bg-background px-1 text-[11px]"
                :aria-label="$t('lead.moveTo')"
                @click.stop
                @keydown.enter.stop
                @change="move(card.id, col.stage.key, $event.target.value)"
              >
                <option v-for="c in columns" :key="c.stage.key" :value="c.stage.key">{{ c.stage.label }}</option>
              </select>
            </div>
          </div>
          <p v-if="!col.cards.length" class="px-1 py-6 text-center text-xs text-muted-foreground">{{ $t('lead.empty') }}</p>
          <p v-else-if="col.total > col.cards.length" class="px-1 text-center text-xs text-muted-foreground">
            {{ $t('lead.more', { n: col.total - col.cards.length }) }}
          </p>
        </div>
      </div>
    </div>

    <AlertDialog :open="!!toDelete" @update:open="(v) => !v && (toDelete = null)">
      <AlertDialogContent>
        <AlertDialogHeader>
          <AlertDialogTitle>{{ $t('lead.delete') }}</AlertDialogTitle>
          <AlertDialogDescription>{{ $t('contact.deleteConfirm') }}</AlertDialogDescription>
        </AlertDialogHeader>
        <AlertDialogFooter>
          <AlertDialogCancel>{{ $t('globals.messages.cancel') }}</AlertDialogCancel>
          <AlertDialogAction variant="destructive" @click="confirmDelete">
            {{ $t('globals.messages.delete') }}
          </AlertDialogAction>
        </AlertDialogFooter>
      </AlertDialogContent>
    </AlertDialog>
  </div>
</template>

<script setup>
import { onMounted, ref, watch } from 'vue'
import { useRouter } from 'vue-router'
import { useI18n } from 'vue-i18n'
import { PhoneMissed, Trash2 } from 'lucide-vue-next'
import { Button } from '@shared-ui/components/ui/button'
import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle
} from '@shared-ui/components/ui/alert-dialog'
import { Input } from '@shared-ui/components/ui/input'
import { getRelativeTime } from '@shared-ui/utils/datetime.js'
import { handleHTTPError } from '@shared-ui/utils/http.js'
import { debounce } from '@shared-ui/utils/debounce.js'
import api from '@main/api'
import { useLeadStore } from '@main/stores/leads'
import { useUserStore } from '@main/stores/user'
import { useEmitter } from '@main/composables/useEmitter'
import { EMITTER_EVENTS } from '@main/constants/emitterEvents.js'

// Contact attributes worth showing on a card, when the workspace has them.
const CARD_ATTRIBUTES = ['course', 'class', 'grade', 'city', 'batch']

const emitter = useEmitter()
const { t } = useI18n()
const router = useRouter()
const leadStore = useLeadStore()
const userStore = useUserStore()
const columns = ref([])
const loading = ref(false)
const search = ref('')
const dragging = ref(null)
const dropTarget = ref(null)
// The lead an admin asked to delete, awaiting confirmation.
const toDelete = ref(null)

const toast = (description) => emitter.emit(EMITTER_EVENTS.SHOW_TOAST, { variant: 'destructive', description })

const load = async () => {
  loading.value = true
  try {
    const resp = await api.getLeadBoard({ q: search.value, per_stage: 100 })
    columns.value = resp.data.data || []
  } catch (error) {
    toast(handleHTTPError(error).message)
  } finally {
    loading.value = false
  }
}

const ago = (ts) => (ts ? getRelativeTime(new Date(ts)) : '')
const kindDot = (kind) => ({ won: 'bg-emerald-500', lost: 'bg-muted-foreground/50' })[kind] || 'bg-sky-500'

const details = (card) => {
  const attrs = card.custom_attributes || {}
  return CARD_ATTRIBUTES.map((k) => attrs[k]).filter((v) => v !== undefined && v !== null && v !== '').map(String)
}

// Moves a card at once and saves in the background, putting it back if the
// save fails.
const move = async (id, from, to) => {
  if (from === to) return
  const src = columns.value.find((c) => c.stage.key === from)
  const dst = columns.value.find((c) => c.stage.key === to)
  const i = src?.cards.findIndex((c) => c.id === id) ?? -1
  if (!src || !dst || i < 0) return
  const [card] = src.cards.splice(i, 1)
  card.stage = to
  card.stage_changed_at = new Date().toISOString()
  dst.cards.unshift(card)
  src.total--
  dst.total++
  try {
    await api.setContactLead(id, to)
  } catch (error) {
    toast(handleHTTPError(error).message)
    load()
  }
}

// Deletes the lead's contact, with their conversations, and takes the card
// off the board.
const confirmDelete = async () => {
  const card = toDelete.value
  toDelete.value = null
  if (!card) return
  try {
    await api.deleteContact(card.id)
  } catch (error) {
    toast(handleHTTPError(error).message)
    return
  }
  for (const col of columns.value) {
    const i = col.cards.findIndex((c) => c.id === card.id)
    if (i >= 0) {
      col.cards.splice(i, 1)
      col.total--
    }
  }
  emitter.emit(EMITTER_EVENTS.SHOW_TOAST, { description: t('globals.messages.deletedSuccessfully') })
}

const conversationRoute = (card) => ({
  name: 'inbox-conversation',
  params: { type: 'all', uuid: card.last_conversation_uuid }
})

// Clicking a card opens the lead's latest conversation, or their contact page
// when they have none yet. A drag doesn't fire a click, so moving still works.
const open = (card) => {
  if (dragging.value) return
  router.push(card.last_conversation_uuid ? conversationRoute(card) : { name: 'contact-detail', params: { id: card.id } })
}

const drop = (to) => {
  dropTarget.value = null
  if (dragging.value) move(dragging.value.id, dragging.value.from, to)
  dragging.value = null
}

watch(search, debounce(load, 300))
// Someone else moved a lead, or a new one came in.
watch(() => leadStore.lastChange, debounce(load, 500))
onMounted(load)
</script>
