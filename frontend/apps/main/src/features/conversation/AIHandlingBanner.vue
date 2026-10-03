<template>
  <!-- An AI assistant is answering this conversation: say so above the reply
       box, and let the agent take over. Replying takes over too. -->
  <div
    v-if="assistant"
    class="flex flex-wrap items-center justify-between gap-2 border-t bg-primary/5 px-4 py-2 text-sm"
  >
    <span class="flex items-center gap-2">
      <Bot class="size-4 shrink-0 text-primary" />
      <span>
        <span class="font-medium">{{ $t('conversation.ai.handling', { name: assistant.label.trim() }) }}</span>
        <span class="text-muted-foreground"> · {{ $t('conversation.ai.replyTakesOver') }}</span>
      </span>
    </span>
    <Button size="sm" variant="outline" class="h-7" :is-loading="busy" :disabled="busy" @click="takeOver">
      {{ $t('conversation.ai.takeOver') }}
    </Button>
  </div>

  <!-- The agent took over from an assistant: offer to hand it back. -->
  <div
    v-else-if="previousAssistant"
    class="flex flex-wrap items-center justify-between gap-2 border-t px-4 py-1.5 text-xs text-muted-foreground"
  >
    <span class="flex items-center gap-2">
      <Bot class="size-3.5 shrink-0" />
      {{ $t('conversation.ai.tookOver', { name: previousAssistant.label.trim() }) }}
    </span>
    <Button size="sm" variant="ghost" class="h-6 text-xs" :is-loading="busy" :disabled="busy" @click="handBack">
      {{ $t('conversation.ai.handBack') }}
    </Button>
  </div>
</template>

<script setup>
import { computed, onMounted, ref, watch } from 'vue'
import { Bot } from 'lucide-vue-next'
import { Button } from '@shared-ui/components/ui/button'
import { useConversationStore } from '@main/stores/conversation'
import { useUserStore } from '@main/stores/user'
import { useUsersStore } from '@main/stores/users'

const conversationStore = useConversationStore()
const userStore = useUserStore()
const usersStore = useUsersStore()
const busy = ref(false)

onMounted(() => usersStore.fetchUsers())

const assistantsByID = computed(() =>
  Object.fromEntries(usersStore.options.filter((u) => u.type === 'ai_assistant').map((u) => [u.value, u]))
)

const conv = computed(() => conversationStore.current)

// The assistant the conversation is assigned to, if it is one.
const assistant = computed(() => assistantsByID.value[String(conv.value?.assigned_user_id ?? '')] || null)

// The assistant that last replied here, when the conversation is now this
// agent's: who "hand back" returns it to.
const previousAssistant = computed(() => {
  if (!conv.value || String(conv.value.assigned_user_id ?? '') !== String(userStore.userID)) return null
  const msgs = conversationStore.conversationMessages || []
  for (let i = msgs.length - 1; i >= 0; i--) {
    const a = assistantsByID.value[String(msgs[i].sender_id)]
    if (a && msgs[i].type === 'outgoing') return a
  }
  return null
})

const assign = async (id) => {
  busy.value = true
  try {
    await conversationStore.updateAssignee('user', { assignee_id: Number(id) })
  } finally {
    busy.value = false
  }
}

const takeOver = () => assign(userStore.userID)
const handBack = () => previousAssistant.value && assign(previousAssistant.value.value)

// Replying takes the conversation over on the server; show it here at once,
// since the reassignment push only reaches this view when the conversation is
// also in the list being shown.
watch(
  () => {
    const msgs = conversationStore.conversationMessages || []
    return msgs.length ? msgs[msgs.length - 1] : null
  },
  (last) => {
    if (assistant.value && last && last.type === 'outgoing' && !last.private && last.sender_id === userStore.userID) {
      conversationStore.current.assigned_user_id = userStore.userID
    }
  }
)
</script>
