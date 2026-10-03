<template>
  <form class="space-y-6 w-full">
    <FormField name="is_active" v-slot="{ value, handleChange }" v-if="!isNewForm">
      <FormItem>
        <FormControl>
          <div class="flex items-center space-x-2">
            <Checkbox :checked="value" @update:checked="handleChange" />
            <Label>{{ $t('globals.terms.active') }}</Label>
          </div>
        </FormControl>
        <FormMessage />
      </FormItem>
    </FormField>

    <FormField name="kind" v-slot="{ value, handleChange }">
      <FormItem>
        <FormLabel>{{ $t('admin.webhook.kind.label') }}</FormLabel>
        <FormControl>
          <div class="grid gap-3 sm:grid-cols-3">
            <button
              v-for="kind in kinds"
              :key="kind.value"
              type="button"
              class="flex flex-col items-start justify-start rounded-md border bg-card p-4 text-left transition-colors hover:bg-muted/40"
              :class="value === kind.value ? 'border-primary ring-1 ring-primary' : 'border-border'"
              :aria-pressed="value === kind.value"
              @click="handleChange(kind.value)"
            >
              <div class="text-sm font-medium text-card-foreground">{{ kind.label }}</div>
              <div class="mt-1 text-xs text-muted-foreground">{{ kind.description }}</div>
            </button>
          </div>
        </FormControl>
        <FormMessage />
      </FormItem>
    </FormField>

    <FormField v-slot="{ componentField }" name="name">
      <FormItem>
        <FormLabel>{{ $t('globals.terms.name') }}</FormLabel>
        <FormControl>
          <Input type="text" placeholder="My Webhook" v-bind="componentField" />
        </FormControl>
        <FormMessage />
      </FormItem>
    </FormField>

    <FormField v-slot="{ componentField }" name="url">
      <FormItem>
        <FormLabel>{{ $t('globals.terms.url') }}</FormLabel>
        <FormControl>
          <Input type="url" :placeholder="urlPlaceholder" v-bind="componentField" />
        </FormControl>
        <FormDescription v-if="urlHelp">{{ urlHelp }}</FormDescription>
        <FormMessage />
      </FormItem>
    </FormField>

    <FormField name="events" v-slot="{ componentField, handleChange }">
      <FormItem>
        <FormLabel>{{ $t('globals.terms.event', 2) }}</FormLabel>
        <FormDescription>
          {{ $t('admin.webhook.events.description') }}
          <template v-if="isChat">{{ $t('admin.webhook.kind.chatEvents') }}</template>
        </FormDescription>
        <FormControl>
          <div class="space-y-6">
            <div
              v-for="group in visibleEvents"
              :key="group.name"
              class="rounded-md border border-border bg-card"
            >
              <div class="border-b border-border bg-muted/30 px-5 py-3">
                <h4 class="font-medium text-card-foreground">{{ group.name }}</h4>
              </div>
              <div class="p-5 grid gap-4 sm:grid-cols-2 lg:grid-cols-3">
                <div
                  v-for="event in group.events"
                  :key="event.value"
                  class="flex items-start space-x-3"
                >
                  <Checkbox
                    :checked="componentField.modelValue?.includes(event.value)"
                    @update:checked="
                      (checked) =>
                        handleEventChange(
                          checked,
                          event.value,
                          handleChange,
                          componentField.modelValue
                        )
                    "
                  />
                  <label class="font-normal text-sm">{{ event.label }}</label>
                </div>
              </div>
            </div>
          </div>
        </FormControl>
        <FormMessage />
      </FormItem>
    </FormField>

    <FormField v-if="!isChat" v-slot="{ componentField }" name="secret">
      <FormItem>
        <FormLabel>{{ $t('globals.terms.secret') }}</FormLabel>
        <FormControl>
          <Input type="password" v-bind="componentField" />
        </FormControl>
        <FormDescription>{{ $t('admin.webhook.secret.description') }}</FormDescription>
        <FormMessage />
      </FormItem>
    </FormField>

    <!-- Form submit button slot -->
    <slot name="footer"></slot>
  </form>
</template>

<script setup>
import { ref, computed, watch } from 'vue'
import { Checkbox } from '@shared-ui/components/ui/checkbox'
import { Label } from '@shared-ui/components/ui/label'
import { useI18n } from 'vue-i18n'
import {
  FormControl,
  FormField,
  FormItem,
  FormLabel,
  FormMessage,
  FormDescription
} from '@shared-ui/components/ui/form'
import { Input } from '@shared-ui/components/ui/input'

const props = defineProps({
  form: {
    type: Object,
    required: true
  },
  isNewForm: {
    type: Boolean
  }
})

const { t } = useI18n()

const kinds = computed(() => [
  {
    value: 'custom',
    label: t('admin.webhook.kind.custom'),
    description: t('admin.webhook.kind.custom.description')
  },
  {
    value: 'slack',
    label: t('admin.webhook.kind.slack'),
    description: t('admin.webhook.kind.slack.description')
  },
  {
    value: 'teams',
    label: t('admin.webhook.kind.teams'),
    description: t('admin.webhook.kind.teams.description')
  }
])

// Events that aren't posted to Slack or Teams, see SkipsChatEvent on the server.
const CHAT_SKIPPED_EVENTS = ['message.updated']

const kind = computed(() => props.form.values.kind || 'custom')
const isChat = computed(() => kind.value === 'slack' || kind.value === 'teams')

const urlPlaceholder = computed(
  () =>
    ({
      slack: 'https://hooks.slack.com/services/...',
      teams: 'https://prod-00.westus.logic.azure.com/workflows/...'
    })[kind.value] || 'https://your-app.com/webhook'
)

const urlHelp = computed(() =>
  isChat.value ? t(`admin.webhook.kind.${kind.value}.urlHelp`) : ''
)

const webhookEvents = ref([
  {
    name: t('globals.terms.conversation'),
    events: [
      {
        value: 'conversation.created',
        label: 'Conversation created'
      },
      {
        value: 'conversation.status_changed',
        label: 'Conversation status changed'
      },
      {
        value: 'conversation.tags_changed',
        label: 'Conversation tags changed'
      },
      {
        value: 'conversation.assigned',
        label: 'Conversation assigned'
      },
      {
        value: 'conversation.unassigned',
        label: 'Conversation unassigned'
      }
    ]
  },
  {
    name: t('globals.terms.message'),
    events: [
      {
        value: 'message.created',
        label: 'Message created'
      },
      {
        value: 'message.updated',
        label: 'Message updated'
      }
    ]
  }
])

const visibleEvents = computed(() =>
  webhookEvents.value.map((group) => ({
    ...group,
    events: isChat.value
      ? group.events.filter((e) => !CHAT_SKIPPED_EVENTS.includes(e.value))
      : group.events
  }))
)

// Drop events chat destinations never post, so they aren't saved unseen.
watch(isChat, (chat) => {
  if (!chat) return
  const events = props.form.values.events || []
  const kept = events.filter((e) => !CHAT_SKIPPED_EVENTS.includes(e))
  if (kept.length !== events.length) props.form.setFieldValue('events', kept)
})

// If checked add event to the list, if unchecked remove it and call handleChange
const handleEventChange = (checked, eventName, handleChange, currentEvents) => {
  const events = currentEvents || []
  handleChange(checked ? [...events, eventName] : events.filter((e) => e !== eventName))
}
</script>
