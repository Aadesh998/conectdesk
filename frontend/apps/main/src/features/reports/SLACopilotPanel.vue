<template>
  <div class="flex flex-col h-full min-h-0">
    <div
      v-if="messages.length"
      class="flex items-center justify-end border-b border-border px-3 py-2"
    >
      <Button
        type="button"
        variant="ghost"
        size="sm"
        class="gap-1.5 text-muted-foreground"
        @click="clearChat"
      >
        <Eraser class="h-3.5 w-3.5" />
        {{ $t('copilot.clearChat') }}
      </Button>
    </div>
    <div ref="scrollRef" class="flex-1 overflow-y-auto p-4 space-y-4 min-h-0">
      <div
        v-if="messages.length === 0"
        class="h-full flex flex-col items-center justify-center gap-4 text-center px-4"
      >
        <div class="h-12 w-12 rounded-full bg-primary/10 flex items-center justify-center">
          <Bot class="h-6 w-6 text-primary" />
        </div>
        <p class="text-xs text-muted-foreground">{{ $t('copilot.sla.emptyState') }}</p>
        <div class="flex flex-col gap-1.5 w-full max-w-[85%]">
          <Button
            v-for="preset in presets"
            :key="preset"
            type="button"
            variant="outline"
            size="sm"
            class="w-full justify-start whitespace-normal h-auto py-1.5 font-normal text-muted-foreground"
            @click="send(preset)"
          >
            {{ preset }}
          </Button>
        </div>
      </div>
      <div
        v-for="(msg, i) in messages"
        :key="i"
        class="flex gap-2"
        :class="msg.isUser ? 'justify-end' : 'justify-start'"
      >
        <div
          v-if="!msg.isUser"
          class="mt-0.5 h-6 w-6 shrink-0 rounded-full bg-primary/10 flex items-center justify-center"
        >
          <Bot class="h-3.5 w-3.5 text-primary" />
        </div>
        <div
          class="flex flex-col gap-1 max-w-[85%]"
          :class="msg.isUser ? 'items-end' : 'items-start'"
        >
          <div
            class="rounded-lg px-3 py-2 text-sm [overflow-wrap:anywhere]"
            :class="
              msg.isUser
                ? 'bg-primary text-primary-foreground whitespace-pre-wrap'
                : 'bg-muted text-foreground'
            "
          >
            <Letter v-if="!msg.isUser" :html="msg.content" class="native-html" />
            <template v-else>{{ msg.content }}</template>
          </div>
          <Tooltip v-if="!msg.isUser && msg.content">
            <TooltipTrigger as-child>
              <Button
                type="button"
                variant="ghost"
                size="icon"
                class="h-6 w-6 text-muted-foreground"
                @click="copyAnswer(msg.content)"
              >
                <Copy class="h-3.5 w-3.5" />
              </Button>
            </TooltipTrigger>
            <TooltipContent>{{ $t('globals.terms.copy') }}</TooltipContent>
          </Tooltip>
        </div>
      </div>
      <div v-if="isThinking" class="flex gap-2 justify-start">
        <div
          class="mt-0.5 h-6 w-6 shrink-0 rounded-full bg-primary/10 flex items-center justify-center"
        >
          <Bot class="h-3.5 w-3.5 text-primary" />
        </div>
        <div class="rounded-lg px-3 py-2 bg-muted text-muted-foreground flex items-center">
          <DotLoader />
          <span class="sr-only">{{ $t('copilot.thinking') }}</span>
        </div>
      </div>
    </div>

    <form class="border-t border-border p-3" @submit.prevent="send">
      <div
        class="rounded-lg border border-input bg-background shadow-sm transition-colors focus-within:ring-1 focus-within:ring-ring"
      >
        <Textarea
          v-model="input"
          :placeholder="$t('copilot.placeholder')"
          rows="2"
          class="min-h-[44px] resize-none border-0 bg-transparent shadow-none focus-visible:ring-0"
          @keydown.enter.exact.prevent="send"
        />
      </div>
    </form>
  </div>
</template>

<script setup>
import { ref, computed, nextTick, onMounted } from 'vue'
import { Button } from '@shared-ui/components/ui/button'
import { Textarea } from '@shared-ui/components/ui/textarea'
import { Tooltip, TooltipContent, TooltipTrigger } from '@shared-ui/components/ui/tooltip'
import { Letter } from 'vue-letter'
import { DotLoader } from '@shared-ui/components/ui/loader'
import { Eraser, Bot, Copy } from 'lucide-vue-next'
import { useEmitter } from '@/composables/useEmitter'
import { EMITTER_EVENTS } from '@/constants/emitterEvents.js'
import { handleHTTPError } from '@shared-ui/utils/http.js'
import { getTextFromHTML } from '@shared-ui/utils/string.js'
import { useI18n } from 'vue-i18n'
import api from '@/api'

const props = defineProps({
  // The dashboard's range in days, 0 for today; answers use it unless the question names another.
  days: { type: Number, required: true }
})

const emitter = useEmitter()
const { t } = useI18n()

const presets = computed(() => [
  t('copilot.sla.preset.overview'),
  t('copilot.sla.preset.worstTeam'),
  t('copilot.sla.preset.atRisk'),
  t('copilot.sla.preset.recentBreaches')
])

const rawMessages = ref([])
const messages = computed(() => rawMessages.value.map((m) => ({ ...m, isUser: m.role === 'user' })))
const input = ref('')
const isThinking = ref(false)
const scrollRef = ref(null)

// A clear bumps the revision so in-flight hydrate/send responses for the old chat are discarded.
let revision = 0

const showError = (error) => {
  emitter.emit(EMITTER_EVENTS.SHOW_TOAST, {
    variant: 'destructive',
    description: handleHTTPError(error).message
  })
}

const scrollToBottom = async () => {
  await nextTick()
  if (scrollRef.value) scrollRef.value.scrollTop = scrollRef.value.scrollHeight
}

// Load the persisted chat so closing the panel or refreshing the page does not lose it.
const hydrate = async () => {
  const rev = revision
  try {
    const resp = await api.getSLACopilotMessages()
    if (rev !== revision || rawMessages.value.length > 0) return
    rawMessages.value = (resp.data.data || []).map((m) => ({ role: m.role, content: m.content }))
    await scrollToBottom()
  } catch {
    // Non-fatal: the panel still works without history.
  }
}

onMounted(hydrate)

const clearChat = async () => {
  revision++
  rawMessages.value = []
  try {
    await api.clearSLACopilotMessages()
  } catch (error) {
    showError(error)
  }
}

const send = async (preset) => {
  const text = (typeof preset === 'string' ? preset : input.value).trim()
  if (!text || isThinking.value) return

  const rev = revision
  rawMessages.value = [...rawMessages.value, { role: 'user', content: text }]
  input.value = ''
  isThinking.value = true
  await scrollToBottom()

  try {
    const resp = await api.aiSLACopilot({ message: text, days: props.days })
    if (rev !== revision) return
    rawMessages.value = [...rawMessages.value, { role: 'assistant', content: resp.data.data || '' }]
  } catch (error) {
    showError(error)
  } finally {
    isThinking.value = false
    await scrollToBottom()
  }
}

const copyAnswer = async (content) => {
  try {
    await navigator.clipboard.writeText(getTextFromHTML(content))
    emitter.emit(EMITTER_EVENTS.SHOW_TOAST, { description: t('globals.messages.copied') })
  } catch {
    emitter.emit(EMITTER_EVENTS.SHOW_TOAST, {
      variant: 'destructive',
      description: t('copilot.copyFailed')
    })
  }
}
</script>
