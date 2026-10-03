<template>
  <form @submit="onSubmit" novalidate class="space-y-6 w-full">
    <!-- The account comes from Meta; it is changed by reconnecting, not edited. -->
    <div class="box p-4 space-y-3">
      <h3 class="font-semibold">{{ $t('admin.inbox.meta.account') }}</h3>

      <dl class="grid grid-cols-[max-content_1fr] gap-x-4 gap-y-1.5 text-sm">
        <template v-if="channel === 'whatsapp'">
          <dt class="text-muted-foreground">{{ $t('globals.terms.whatsapp') }}</dt>
          <dd>
            {{ config.display_phone_number }}
            <span v-if="config.verified_name" class="text-muted-foreground">
              · {{ config.verified_name }}
            </span>
          </dd>
          <dt class="text-muted-foreground">{{ $t('admin.inbox.meta.waba') }}</dt>
          <dd class="font-mono text-xs">{{ config.waba_id }}</dd>
        </template>
        <template v-else-if="channel === 'telegram'">
          <dt class="text-muted-foreground">{{ $t('telegram.bot') }}</dt>
          <dd>@{{ config.username }}</dd>
          <dt class="text-muted-foreground">{{ $t('telegram.business') }}</dt>
          <dd>
            <template v-if="config.business">
              {{ config.business.name }}
              <span v-if="config.business.username" class="text-muted-foreground">@{{ config.business.username }}</span>
              <span v-if="!config.business.can_reply" class="text-destructive"> · {{ $t('telegram.business.cantReply') }}</span>
            </template>
            <span v-else class="text-muted-foreground">{{ $t('telegram.business.none') }}</span>
          </dd>
        </template>
        <template v-else>
          <template v-if="channel === 'instagram'">
            <dt class="text-muted-foreground">{{ $t('globals.terms.instagram') }}</dt>
            <dd>@{{ config.username }}</dd>
          </template>
          <dt class="text-muted-foreground">{{ $t('admin.inbox.meta.page') }}</dt>
          <dd>{{ config.page_name }}</dd>
        </template>
      </dl>

      <p v-if="config.disconnected" class="text-sm text-destructive">
        {{ $t('admin.inbox.meta.disconnected') }}
      </p>
      <p v-if="config.registration_error" class="text-sm text-destructive">
        {{ $t('admin.inbox.meta.registrationError', { error: config.registration_error }) }}
      </p>
      <p v-if="channel !== 'telegram'" class="text-xs text-muted-foreground">{{ $t('admin.inbox.meta.window') }}</p>

      <TelegramWebhookStatus v-if="channel === 'telegram' && initialValues.id" :inbox-id="initialValues.id" />
      <!-- A number connected with the business's own Meta app is renewed by
           entering its credentials again, not through Facebook. -->
      <template v-else-if="manual">
        <WhatsAppWebhookInfo v-if="initialValues.id" :key="webhookKey" :inbox-id="initialValues.id" />
        <Button v-if="!showReconnect" type="button" variant="outline" @click="showReconnect = true">
          {{ $t('whatsapp.manual.updateTitle') }}
        </Button>
        <WhatsAppManualConnect v-else reconnect @connected="onReconnected" />
      </template>
      <template v-else>
        <p class="text-xs text-muted-foreground">
          {{ $t('admin.inbox.meta.reconnect.description') }}
        </p>
        <MetaConnectButton
          :channel="channel"
          variant="outline"
          :label="$t('admin.inbox.meta.reconnect')"
        />
      </template>
    </div>

    <WhatsAppHealth
      v-if="channel === 'whatsapp' && initialValues.id"
      :inbox-id="initialValues.id"
      :alert="config.health_alert || null"
    />
    <WhatsAppTemplateManager v-if="channel === 'whatsapp' && initialValues.id" :inbox-id="initialValues.id" />
    <MetaHistoryImport
      v-if="(channel === 'messenger' || channel === 'instagram') && initialValues.id"
      :inbox-id="initialValues.id"
    />

    <FormField v-slot="{ componentField }" name="name">
      <FormItem>
        <FormLabel>{{ $t('globals.terms.name') }}</FormLabel>
        <FormControl>
          <Input type="text" v-bind="componentField" />
        </FormControl>
        <FormMessage />
      </FormItem>
    </FormField>

    <!-- New conversations on this inbox go straight to an AI assistant. -->
    <div class="box p-4 space-y-2">
      <FormField v-slot="{ componentField }" name="config.ai_assistant_id">
        <FormItem>
          <FormLabel>{{ $t('admin.inbox.aiAssistant') }}</FormLabel>
          <FormControl>
            <select
              :value="componentField.modelValue"
              class="w-full max-w-sm rounded-md border bg-background px-2 py-2 text-sm"
              @change="componentField['onUpdate:modelValue'](Number($event.target.value))"
            >
              <option :value="0">{{ $t('admin.inbox.aiAssistant.none') }}</option>
              <option v-for="a in assistants" :key="a.id" :value="a.id">{{ a.name }}</option>
            </select>
          </FormControl>
          <FormDescription>{{ $t('admin.inbox.aiAssistant.description') }}</FormDescription>
          <FormMessage />
        </FormItem>
      </FormField>
    </div>

    <div class="box p-4 space-y-4">
      <FormField v-slot="{ componentField }" name="config.max_media_size_mb">
        <FormItem>
          <FormLabel>{{ $t('admin.inbox.whatsapp.maxMediaSize') }}</FormLabel>
          <FormControl>
            <Input type="number" placeholder="20" v-bind="componentField" />
          </FormControl>
          <FormDescription>
            {{ $t('admin.inbox.whatsapp.maxMediaSize.description') }}
          </FormDescription>
          <FormMessage />
        </FormItem>
      </FormField>

      <FormField
        v-if="channel === 'telegram'"
        v-slot="{ componentField, handleChange }"
        name="config.allow_groups"
      >
        <FormItem>
          <SwitchField
            :title="$t('telegram.allowGroups')"
            :description="$t('telegram.allowGroups.description')"
            :checked="componentField.modelValue"
            @update:checked="handleChange"
          />
        </FormItem>
      </FormField>

      <FormField
        v-if="channel === 'whatsapp'"
        v-slot="{ componentField, handleChange }"
        name="config.mark_seen"
      >
        <FormItem>
          <SwitchField
            :title="$t('admin.inbox.whatsapp.markSeen')"
            :description="$t('admin.inbox.whatsapp.markSeen.description')"
            :checked="componentField.modelValue"
            @update:checked="handleChange"
          />
        </FormItem>
      </FormField>
    </div>

    <!-- Inbox behaviour, shared with the other channels -->
    <FormField v-slot="{ componentField, handleChange }" name="enabled">
      <FormItem>
        <SwitchField
          :title="$t('globals.terms.enabled')"
          :description="$t('admin.inbox.enabled.description')"
          :checked="componentField.modelValue"
          @update:checked="handleChange"
        />
      </FormItem>
    </FormField>

    <FormField v-slot="{ componentField, handleChange }" name="csat_enabled">
      <FormItem>
        <SwitchField
          :title="$t('admin.inbox.csatSurveys')"
          :description="$t('admin.inbox.csatSurveys.description_1')"
          :checked="componentField.modelValue"
          @update:checked="handleChange"
        />
      </FormItem>
    </FormField>

    <FormField v-slot="{ componentField, handleChange }" name="prompt_tags_on_reply">
      <FormItem>
        <SwitchField
          :title="$t('admin.inbox.promptTagsOnReply')"
          :description="$t('admin.inbox.promptTagsOnReply.description')"
          :checked="componentField.modelValue"
          @update:checked="handleChange"
        />
      </FormItem>
    </FormField>

    <Button type="submit" :is-loading="isLoading" :disabled="isLoading">
      {{ $t('globals.messages.save') }}
    </Button>
  </form>
</template>

<script setup>
import { computed, onMounted, ref, watch } from 'vue'
import { useForm } from 'vee-validate'
import { toTypedSchema } from '@vee-validate/zod'
import { z } from 'zod'
import {
  FormControl,
  FormField,
  FormItem,
  FormLabel,
  FormMessage,
  FormDescription
} from '@shared-ui/components/ui/form/index.js'
import { Input } from '@shared-ui/components/ui/input/index.js'
import { Button } from '@shared-ui/components/ui/button/index.js'
import SwitchField from '@shared-ui/components/SwitchField.vue'
import MetaConnectButton from './MetaConnectButton.vue'
import WhatsAppTemplateManager from './WhatsAppTemplateManager.vue'
import MetaHistoryImport from './MetaHistoryImport.vue'
import WhatsAppHealth from './WhatsAppHealth.vue'
import TelegramWebhookStatus from './TelegramWebhookStatus.vue'
import WhatsAppManualConnect from './WhatsAppManualConnect.vue'
import WhatsAppWebhookInfo from './WhatsAppWebhookInfo.vue'
import { useI18n } from 'vue-i18n'
import { useEmitter } from '@main/composables/useEmitter'
import api from '@/api'
import { EMITTER_EVENTS } from '@main/constants/emitterEvents.js'

const props = defineProps({
  initialValues: {
    type: Object,
    default: () => ({})
  },
  submitForm: {
    type: Function,
    required: true
  },
  isLoading: {
    type: Boolean,
    default: false
  }
})

const { t } = useI18n()
const emitter = useEmitter()

// The server caps this again against the workspace's upload limit.
const MAX_MEDIA_SIZE_MB = 100

const schema = computed(() =>
  toTypedSchema(
    z.object({
      name: z.string().min(1, { message: t('globals.messages.required') }),
      enabled: z.boolean(),
      csat_enabled: z.boolean(),
      prompt_tags_on_reply: z.boolean(),
      config: z.object({
        max_media_size_mb: z.coerce
          .number()
          .min(1, { message: t('validation.minmaxNumber', { min: 1, max: MAX_MEDIA_SIZE_MB }) })
          .max(MAX_MEDIA_SIZE_MB, {
            message: t('validation.minmaxNumber', { min: 1, max: MAX_MEDIA_SIZE_MB })
          }),
        mark_seen: z.boolean().optional(),
        allow_groups: z.boolean().optional(),
        ai_assistant_id: z.coerce.number().int().min(0).optional()
      })
    })
  )
)

const form = useForm({
  validationSchema: schema,
  initialValues: {
    name: '',
    enabled: true,
    csat_enabled: false,
    prompt_tags_on_reply: false,
    config: { max_media_size_mb: 20, mark_seen: true, allow_groups: false, ai_assistant_id: 0 }
  }
})

const channel = computed(() => props.initialValues.channel)
const config = computed(() => props.initialValues.config || {})
// Enabled assistants new conversations can go to.
const assistants = ref([])
onMounted(async () => {
  try {
    assistants.value = (await api.getAIAssistantsCompact()).data.data || []
  } catch {
    assistants.value = []
  }
})

const manual = computed(() => channel.value === 'whatsapp' && config.value.connection_mode === 'manual')
const showReconnect = ref(false)
const webhookKey = ref(0)

const onReconnected = () => {
  showReconnect.value = false
  webhookKey.value++
  emitter.emit(EMITTER_EVENTS.SHOW_TOAST, { description: t('toast.inboxConnected') })
}

// Only the options travel back; the server keeps the account and its token.
const onSubmit = form.handleSubmit(async (values) => {
  await props.submitForm({
    ...values,
    config: {
      max_media_size_mb: values.config.max_media_size_mb,
      mark_seen: !!values.config.mark_seen,
      allow_groups: !!values.config.allow_groups,
      ai_assistant_id: Number(values.config.ai_assistant_id) || 0
    }
  })
})

watch(
  () => props.initialValues,
  (v) => {
    if (!v || Object.keys(v).length === 0) return
    form.setValues(
      {
        name: v.name,
        enabled: v.enabled,
        csat_enabled: v.csat_enabled,
        prompt_tags_on_reply: v.prompt_tags_on_reply,
        config: {
          max_media_size_mb: v.config?.max_media_size_mb || 20,
          mark_seen: v.config?.mark_seen ?? true,
          allow_groups: !!v.config?.allow_groups,
          ai_assistant_id: Number(v.config?.ai_assistant_id) || 0
        }
      },
      false
    )
  },
  { deep: true, immediate: true }
)
</script>
