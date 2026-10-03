<template>
  <div>
    <Dialog v-model:open="dialogOpen">
      <DialogContent class="max-w-5xl h-[90vh] flex flex-col" >
        <DialogHeader>
          <DialogTitle>
            {{ $t('conversation.newConversation') }}
          </DialogTitle>
          <DialogDescription>
            <!-- A number can be called straight away, without writing first. -->
            <span v-if="hasVoiceInbox" class="inline-flex items-center gap-1.5">
              {{ $t('voice.dialer.orDial') }}
              <button type="button" class="inline-flex items-center gap-1 font-medium text-foreground underline" @click="openDialer">
                <PhoneOutgoing :size="13" />
                {{ $t('voice.makeCall') }}
              </button>
            </span>
          </DialogDescription>
        </DialogHeader>

        <form @submit="createConversation" novalidate class="flex flex-col flex-1 overflow-hidden">
          <!-- Form Fields Section -->
          <div class="space-y-4 pb-2 flex-shrink-0">
            <div class="space-y-2">
              <!-- WhatsApp and phone calls address a contact by number; there is no email to search on. -->
              <FormField v-if="byPhone" v-slot="{ componentField }" name="contact_phone_number">
                <FormItem>
                  <FormLabel>{{ isVoice ? $t('voice.phoneNumber') : $t('conversation.whatsappNumber') }}</FormLabel>
                  <FormControl>
                    <Input
                      type="tel"
                      :placeholder="isVoice ? t('voice.phoneNumber.placeholder') : t('conversation.whatsappNumber.placeholder')"
                      v-bind="componentField"
                      autocomplete="off"
                    />
                  </FormControl>
                  <FormMessage />
                </FormItem>
              </FormField>

              <FormField v-else name="contact_email">
                <FormItem class="relative">
                  <FormLabel>{{ $t('globals.terms.email') }}</FormLabel>
                  <FormControl>
                    <Input
                      ref="emailInputRef"
                      type="email"
                      :placeholder="t('conversation.searchContact')"
                      v-model="emailQuery"
                      @input="handleSearchContacts"
                      @keydown="handleSearchKeydown"
                      autocomplete="off"
                    />
                  </FormControl>
                  <FormMessage />

                  <div
                    v-if="searchResults.length"
                    class="absolute w-full z-50 mt-1 rounded-md border bg-popover p-1 text-popover-foreground shadow-md"
                  >
                    <ul class="max-h-60 overflow-y-auto" role="listbox">
                      <li
                        v-for="(contact, index) in searchResults"
                        :key="contact.email"
                        @click="selectContact(contact)"
                        role="option"
                        :aria-selected="index === highlightedIndex"
                        class="relative flex cursor-pointer select-none items-center rounded-sm px-2 py-1.5 text-sm outline-none transition-colors duration-200"
                        :class="
                          index === highlightedIndex
                            ? 'bg-accent text-accent-foreground'
                            : 'hover:bg-accent hover:text-accent-foreground'
                        "
                      >
                        <div>
                          <p class="font-medium">
                            {{ contact.first_name }} {{ contact.last_name }}
                          </p>
                          <p class="text-xs text-muted-foreground">{{ contact.email }}</p>
                          <div
                            v-if="contact.external_user_id"
                            class="flex items-center gap-1 text-xs text-muted-foreground"
                          >
                            <IdCard :size="12" class="flex-shrink-0" />
                            <span class="truncate">{{ contact.external_user_id }}</span>
                          </div>
                        </div>
                      </li>
                    </ul>
                  </div>
                </FormItem>
              </FormField>

              <!-- Name Group -->
              <div class="grid grid-cols-2 gap-4">
                <FormField v-slot="{ componentField }" name="first_name">
                  <FormItem>
                    <FormLabel>{{ $t('globals.terms.firstName') }}</FormLabel>
                    <FormControl>
                      <Input
                        type="text"
                        placeholder=""
                        v-bind="componentField"
                        :disabled="!!selectedContact"
                        required
                      />
                    </FormControl>
                    <FormMessage />
                  </FormItem>
                </FormField>

                <FormField v-slot="{ componentField }" name="last_name">
                  <FormItem>
                    <FormLabel>{{ $t('globals.terms.lastName') }}</FormLabel>
                    <FormControl>
                      <Input
                        type="text"
                        placeholder=""
                        v-bind="componentField"
                        :disabled="!!selectedContact"
                      />
                    </FormControl>
                    <FormMessage />
                  </FormItem>
                </FormField>
              </div>

              <!-- Subject and Inbox Group -->
              <div class="grid grid-cols-2 gap-4">
                <FormField v-if="!byPhone" v-slot="{ componentField }" name="subject">
                  <FormItem>
                    <FormLabel>{{ $t('globals.terms.subject') }}</FormLabel>
                    <FormControl>
                      <Input type="text" placeholder="" v-bind="componentField" />
                    </FormControl>
                    <FormMessage />
                  </FormItem>
                </FormField>

                <FormField v-slot="{ componentField }" name="inbox_id">
                  <FormItem>
                    <FormLabel>{{ $t('globals.terms.inbox') }}</FormLabel>
                    <FormControl>
                      <Select v-bind="componentField">
                        <SelectTrigger>
                          <SelectValue :placeholder="t('placeholders.selectInbox')" />
                        </SelectTrigger>
                        <SelectContent>
                          <SelectGroup>
                            <SelectItem
                              v-for="option in inboxStore.outboundOptions"
                              :key="option.value"
                              :value="option.value"
                            >
                              {{ option.label }}
                            </SelectItem>
                          </SelectGroup>
                        </SelectContent>
                      </Select>
                    </FormControl>
                    <FormMessage />
                  </FormItem>
                </FormField>
              </div>

              <!-- Assignment Group -->
              <div class="grid grid-cols-2 gap-4">
                <!-- Set assigned team -->
                <FormField v-slot="{ componentField }" name="team_id">
                  <FormItem>
                    <FormLabel>
                      {{ $t('actions.assignTeam') }}
                      ({{ $t('globals.terms.optional') }})
                    </FormLabel>
                    <FormControl>
                      <SelectComboBox
                        v-bind="componentField"
                        :items="[
                          { value: 'none', label: t('globals.terms.none') },
                          ...teamStore.options
                        ]"
                        :placeholder="t('placeholders.selectTeam')"
                        type="team"
                      />
                    </FormControl>
                    <FormMessage />
                  </FormItem>
                </FormField>

                <!-- Set assigned agent -->
                <FormField v-slot="{ componentField }" name="agent_id">
                  <FormItem>
                    <FormLabel>
                      {{ $t('actions.assignAgent') }}
                      ({{ $t('globals.terms.optional') }})
                    </FormLabel>
                    <FormControl>
                      <SelectComboBox
                        v-bind="componentField"
                        :items="[
                          { value: 'none', label: t('globals.terms.none') },
                          ...uStore.options
                        ]"
                        :placeholder="t('placeholders.selectAgent')"
                        type="user"
                      />
                    </FormControl>
                    <FormMessage />
                  </FormItem>
                </FormField>
              </div>
            </div>
          </div>

          <!-- A phone conversation opens with a call from the agent's phone. -->
          <div v-if="isVoice" class="mt-4 flex-shrink-0">
            <div class="box flex items-start gap-3 p-4 text-sm">
              <PhoneOutgoing :size="18" class="mt-0.5 shrink-0 text-muted-foreground" />
              <p v-if="userStore.phoneNumber" class="text-muted-foreground">
                {{ $t('voice.newCallHint', { phone: userStore.phoneNumber }) }}
              </p>
              <p v-else class="text-destructive">
                {{ $t('voice.newCallNoPhone') }}
                <router-link :to="{ name: 'profile' }" class="underline" @click="dialogOpen = false">
                  {{ $t('globals.terms.profile') }}
                </router-link>
              </p>
            </div>
          </div>

          <!-- A WhatsApp conversation can only open with an approved template. -->
          <div v-else-if="isWhatsApp" class="flex-1 min-h-0 mt-4 overflow-y-auto">
            <WhatsAppTemplatePicker v-model="whatsAppTemplate" :inbox-id="selectedInboxID" />
          </div>

          <!-- Message Editor Section -->
          <div v-else class="flex-1 flex flex-col min-h-0 mt-4">
            <FormField v-slot="{ componentField }" name="content">
              <FormItem class="flex flex-col h-full">
                <FormLabel>{{ $t('globals.terms.message') }}</FormLabel>
                <FormControl class="flex-1 flex flex-col min-h-0">
                  <div class="flex flex-col h-full">
                    <Editor
                      v-model:htmlContent="componentField.modelValue"
                      @update:htmlContent="(value) => componentField.onChange(value)"
                      :placeholder="isCramped ? t('globals.terms.typeMessage') : t('editor.hint.newLineCtrlK')"
                      :insertContent="insertContent"
                      :autoFocus="false"
                      :enableInlineImages="true"
                      class="w-full flex-1 overflow-y-auto p-2 box min-h-0"
                      @send="createConversation"
                      @filesDropped="uploadFiles"
                    />

                    <MacroActionsPreview
                      v-if="
                        conversationStore.getMacro(MACRO_CONTEXT.NEW_CONVERSATION).actions?.length >
                        0
                      "
                      :actions="
                        conversationStore.getMacro(MACRO_CONTEXT.NEW_CONVERSATION)?.actions || []
                      "
                      :onRemove="
                        (action) =>
                          conversationStore.removeMacroAction(
                            action,
                            MACRO_CONTEXT.NEW_CONVERSATION
                          )
                      "
                      class="mt-2 flex-shrink-0"
                    />

                    <ReplyBoxAttachmentPreview
                      :attachments="mediaFiles"
                      :uploadingFiles="uploadingFiles"
                      :onDelete="handleFileDelete"
                      v-if="mediaFiles.length > 0 || uploadingFiles.length > 0"
                      class="mt-2 flex-shrink-0"
                    />
                  </div>
                </FormControl>
                <FormMessage />
              </FormItem>
            </FormField>
          </div>

          <DialogFooter class="mt-4 pt-2 flex items-center !justify-between w-full flex-shrink-0">
            <ReplyBoxMenuBar
              v-if="!byPhone"
              :handleFileUpload="handleFileUpload"
              @emojiSelect="handleEmojiSelect"
              :showSendButton="false"
              :showGenerateReply="false"
            />
            <span v-else />
            <Button type="submit" :disabled="isDisabled" :isLoading="loading" class="gap-1.5">
              <template v-if="isVoice">
                <Phone :size="14" />
                {{ $t('voice.startCall') }}
              </template>
              <template v-else>{{ $t('globals.messages.submit') }}</template>
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  </div>
</template>

<script setup>
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogFooter,
  DialogDescription
} from '@shared-ui/components/ui/dialog'
import { Button } from '@shared-ui/components/ui/button'
import { Input } from '@shared-ui/components/ui/input'
import { useForm } from 'vee-validate'
import { toTypedSchema } from '@vee-validate/zod'
import {
  FormControl,
  FormField,
  FormItem,
  FormLabel,
  FormMessage
} from '@shared-ui/components/ui/form'
import { z } from 'zod'
import { ref, watch, onUnmounted, nextTick, onMounted, computed } from 'vue'
import ReplyBoxAttachmentPreview from '@/features/conversation/message/attachment/ReplyBoxAttachmentPreview.vue'
import { useConversationStore } from '../../stores/conversation'
import MacroActionsPreview from '@/features/conversation/MacroActionsPreview.vue'
import ReplyBoxMenuBar from '@/features/conversation/ReplyBoxMenuBar.vue'
import { EMITTER_EVENTS } from '@main/constants/emitterEvents.js'
import { MACRO_CONTEXT } from '@main/constants/conversation'
import { useEmitter } from '@main/composables/useEmitter'
import { handleHTTPError } from '@shared-ui/utils/http.js'
import { useInboxStore } from '@main/stores/inbox'
import { useUsersStore } from '@main/stores/users'
import { useTeamStore } from '@main/stores/team'
import {
  Select,
  SelectContent,
  SelectGroup,
  SelectItem,
  SelectTrigger,
  SelectValue
} from '@shared-ui/components/ui/select'
import { useI18n } from 'vue-i18n'
import { useFileUpload } from '@/composables/useFileUpload'
import Editor from '@/components/editor/ConversationEditor.vue'
import { useMacroStore } from '@/stores/macro'
import SelectComboBox from '@/components/combobox/SelectCombobox.vue'
import { UserTypeAgent } from '@/constants/user'
import { IdCard, Phone, PhoneOutgoing } from 'lucide-vue-next'
import { useUserStore } from '@main/stores/user'
import api from '@/api'
import WhatsAppTemplatePicker from '@/features/conversation/whatsapp/WhatsAppTemplatePicker.vue'
import { hasPendingInlineUpload } from '@main/composables/useInlineImageUpload'
import { useIsComposerCramped } from '@main/composables/useIsComposerCramped'

const dialogOpen = defineModel({
  required: false,
  default: () => false
})

const inboxStore = useInboxStore()
const userStore = useUserStore()
const { t } = useI18n()
const uStore = useUsersStore()
const teamStore = useTeamStore()
const emitter = useEmitter()
const isCramped = useIsComposerCramped()
const loading = ref(false)
const searchResults = ref([])
const emailQuery = ref('')
const conversationStore = useConversationStore()
const macroStore = useMacroStore()
let timeoutId = null
let previousMacroView = ''
const insertContent = ref('')
const selectedContact = ref(null)
const emailInputRef = ref(null)

const handleEmojiSelect = (emoji) => {
  insertContent.value = undefined
  // Force reactivity so the user can select the same emoji multiple times
  nextTick(() => (insertContent.value = emoji))
}

const {
  uploadingFiles,
  handleFileUpload,
  handleFileDelete,
  uploadFiles,
  mediaFiles,
  clearMediaFiles
} = useFileUpload({
  linkedModel: 'messages'
})

const isDisabled = computed(() => {
  if (loading.value || uploadingFiles.value.length > 0) return true
  if (hasPendingInlineUpload(form?.values?.content)) return true
  if (isWhatsApp.value && !whatsAppTemplate.value) return true
  if (isVoice.value && !userStore.phoneNumber) return true
  return false
})

// The selected inbox's channel decides how a contact is addressed, so the schema
// is rebuilt when it changes: WhatsApp wants a phone number and a template,
// email an address, a subject and a message.
// Mirrored from the form rather than read off it, because the schema below is
// built before useForm() exists.
const selectedInboxID = ref(null)
const selectedChannel = computed(
  () =>
    inboxStore.outboundOptions.find((option) => option.value === String(selectedInboxID.value))
      ?.channel
)
const isWhatsApp = computed(() => selectedChannel.value === 'whatsapp')
// A Phone inbox opens the conversation by calling the customer.
const isVoice = computed(() => selectedChannel.value === 'voice')
const hasVoiceInbox = computed(() => inboxStore.outboundOptions.some((o) => o.channel === 'voice'))
const openDialer = () => {
  dialogOpen.value = false
  emitter.emit(EMITTER_EVENTS.OPEN_DIALER)
}
const byPhone = computed(() => isWhatsApp.value || isVoice.value)
const whatsAppTemplate = ref(null)

// A WhatsApp number is digits with the country code, allowing the spaces,
// brackets and dashes people paste in.
const whatsAppNumberPattern = /^\+?[\d\s()-]{8,20}$/

const formSchema = computed(() =>
  z.object({
    subject: byPhone.value
      ? z.string().optional()
      : z.string().min(1, t('validation.subjectCannotBeEmpty')),
    content: byPhone.value
      ? z.string().optional()
      : z.string().min(1, t('validation.messageCannotBeEmpty')),
    inbox_id: z
      .any()
      .refine((val) => inboxStore.outboundOptions.some((option) => option.value === val), {
        message: t('globals.messages.required')
      }),
    team_id: z.any().optional(),
    agent_id: z.any().optional(),
    contact_email: byPhone.value
      ? z.string().optional()
      : z.string().email(t('validation.invalidEmail')),
    contact_phone_number: byPhone.value
      ? z
          .string()
          .regex(
            whatsAppNumberPattern,
            isVoice.value ? t('voice.invalidAgentPhone') : t('whatsapp.invalidPhoneNumber')
          )
      : z.string().optional(),
    first_name: z.string().min(1, t('globals.messages.required')),
    last_name: z.string().optional()
  })
)

onUnmounted(() => {
  clearTimeout(timeoutId)
  clearMediaFiles()
  conversationStore.resetMacro(MACRO_CONTEXT.NEW_CONVERSATION)
  macroStore.setCurrentView(previousMacroView)
  emitter.emit(EMITTER_EVENTS.SET_NESTED_COMMAND, {
    command: null,
    open: false
  })
})

onMounted(() => {
  previousMacroView = macroStore.currentView
  macroStore.setCurrentView('starting_conversation')
  emitter.emit(EMITTER_EVENTS.SET_NESTED_COMMAND, {
    command: 'apply-macro-to-new-conversation',
    open: false
  })
  nextTick(() => {
    emailInputRef.value?.$el?.focus()
  })
})

const form = useForm({
  validationSchema: computed(() => toTypedSchema(formSchema.value)),
  initialValues: {
    inbox_id: null,
    team_id: null,
    agent_id: null,
    subject: '',
    content: '',
    contact_email: '',
    contact_phone_number: '',
    first_name: '',
    last_name: ''
  }
})

watch(emailQuery, (newVal) => {
  form.setFieldValue('contact_email', newVal)
  if (selectedContact.value && newVal !== selectedContact.value.email) {
    selectedContact.value = null
    form.setFieldValue('first_name', '')
    form.setFieldValue('last_name', '')
  }
})

const handleSearchContacts = async () => {
  clearTimeout(timeoutId)
  timeoutId = setTimeout(async () => {
    const query = emailQuery.value.trim()

    if (query.length < 3) {
      searchResults.value.splice(0)
      return
    }

    try {
      const resp = await api.searchContacts({ query })
      searchResults.value = [...resp.data.data]
      highlightedIndex.value = -1
    } catch (error) {
      emitter.emit(EMITTER_EVENTS.SHOW_TOAST, {
        variant: 'destructive',
        description: handleHTTPError(error).message
      })
      searchResults.value.splice(0)
    }
  }, 300)
}

const highlightedIndex = ref(-1)

const handleSearchKeydown = (e) => {
  if (!searchResults.value.length) return
  if (e.key === 'ArrowDown') {
    e.preventDefault()
    highlightedIndex.value = Math.min(highlightedIndex.value + 1, searchResults.value.length - 1)
  } else if (e.key === 'ArrowUp') {
    e.preventDefault()
    highlightedIndex.value = Math.max(highlightedIndex.value - 1, 0)
  } else if (e.key === 'Enter' && highlightedIndex.value >= 0) {
    e.preventDefault()
    selectContact(searchResults.value[highlightedIndex.value])
  } else if (e.key === 'Escape') {
    searchResults.value.splice(0)
    highlightedIndex.value = -1
  }
}

const selectContact = (contact) => {
  selectedContact.value = contact
  emailQuery.value = contact.email
  form.setFieldValue('first_name', contact.first_name)
  form.setFieldValue('last_name', contact.last_name || '')
  searchResults.value.splice(0)
  highlightedIndex.value = -1
}

watch(
  () => form.values.inbox_id,
  (id) => {
    selectedInboxID.value = id
    whatsAppTemplate.value = null
  }
)

const createConversation = form.handleSubmit(async (values) => {
  if (isWhatsApp.value) {
    if (!whatsAppTemplate.value) {
      emitter.emit(EMITTER_EVENTS.SHOW_TOAST, {
        variant: 'destructive',
        description: t('whatsapp.templateRequired')
      })
      return
    }
    values.whatsapp_template = whatsAppTemplate.value
    values.content = ''
    values.contact_email = ''
  }
  if (isVoice.value) {
    values.content = ''
    values.contact_email = ''
  }
  loading.value = true
  try {
    // Convert ids to numbers if they are not already
    values.inbox_id = Number(values.inbox_id)
    values.team_id = values.team_id ? Number(values.team_id) : null
    values.agent_id = values.agent_id ? Number(values.agent_id) : null
    // Array of attachment ids.
    values.attachments = isWhatsApp.value
      ? values.whatsapp_template.attachments
      : isVoice.value
        ? []
        : mediaFiles.value.map((file) => file.id)
    if (selectedContact.value?.external_user_id) {
      values.external_user_id = selectedContact.value.external_user_id
    }
    // Form data is a snapshot from search; never let it overwrite the stored contact.
    values.reuse_contact = true
    // Initiator of this conversation is always agent
    values.initiator = UserTypeAgent
    const conversation = await api.createConversation(values)
    const conversationUUID = conversation.data.data.uuid

    // Get macro from context, and set if any actions are available.
    const macro = conversationStore.getMacro(MACRO_CONTEXT.NEW_CONVERSATION)
    if (conversationUUID !== '' && macro?.id) {
      try {
        await api.applyMacro(conversationUUID, macro.id, macro.actions)
      } catch (error) {
        emitter.emit(EMITTER_EVENTS.SHOW_TOAST, {
          variant: 'destructive',
          description: handleHTTPError(error).message
        })
      }
    }
    if (isVoice.value) {
      emitter.emit(EMITTER_EVENTS.SHOW_TOAST, { description: t('voice.callStarted') })
    }
    dialogOpen.value = false
    form.resetForm()
  } catch (error) {
    emitter.emit(EMITTER_EVENTS.SHOW_TOAST, {
      variant: 'destructive',
      description: handleHTTPError(error).message
    })
  } finally {
    loading.value = false
  }
})

/**
 * Watches for changes in the macro id and update message content.
 */
watch(
  () => conversationStore.getMacro(MACRO_CONTEXT.NEW_CONVERSATION).id,
  () => {
    const content = conversationStore.getMacro(MACRO_CONTEXT.NEW_CONVERSATION).message_content
    if (content) form.setFieldValue('content', content)
  },
  { deep: true }
)
</script>
