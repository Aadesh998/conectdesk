<template>
  <div class="rounded-md border border-border bg-card">
    <div class="flex flex-wrap items-center justify-between gap-3 border-b border-border px-5 py-3">
      <div class="min-w-0">
        <div class="truncate font-medium text-card-foreground">{{ channelLabel }}</div>
        <div class="truncate text-xs text-muted-foreground">
          {{ connection.external_team_name || connection.external_team_id }}
        </div>
      </div>
      <div class="flex items-center gap-2">
        <Switch :checked="isActive" @update:checked="(v) => (isActive = v)" />
        <span class="text-sm">
          {{ isActive ? $t('globals.terms.active') : $t('globals.terms.inactive') }}
        </span>
      </div>
    </div>

    <div class="space-y-4 p-5">
      <div>
        <div class="mb-2 text-sm font-medium">{{ $t('admin.chat.postsLabel') }}</div>
        <div class="grid gap-3 sm:grid-cols-2">
          <label v-for="event in events" :key="event" class="flex items-start gap-3 text-sm">
            <Checkbox
              :checked="selected.includes(event)"
              @update:checked="(checked) => toggle(event, checked)"
            />
            <span>{{ $t(`admin.chat.event.${event}`) }}</span>
          </label>
        </div>
      </div>

      <div class="flex flex-wrap gap-2">
        <Button size="sm" :is-loading="saving" :disabled="selected.length === 0" @click="save">
          {{ $t('globals.messages.save') }}
        </Button>
        <Button size="sm" variant="outline" :is-loading="testing" @click="test">
          {{ $t('webhook.sendTest') }}
        </Button>
        <Button size="sm" variant="ghost" class="text-destructive" @click="confirmOpen = true">
          {{ $t('admin.chat.disconnect') }}
        </Button>
      </div>
    </div>
  </div>

  <AlertDialog :open="confirmOpen" @update:open="confirmOpen = $event">
    <AlertDialogContent>
      <AlertDialogHeader>
        <AlertDialogTitle>{{ $t('admin.chat.disconnectTitle', { channel: channelLabel }) }}</AlertDialogTitle>
        <AlertDialogDescription>{{ $t(`admin.chat.${connection.provider}.disconnectHelp`) }}</AlertDialogDescription>
      </AlertDialogHeader>
      <AlertDialogFooter>
        <AlertDialogCancel>{{ $t('globals.messages.cancel') }}</AlertDialogCancel>
        <AlertDialogAction variant="destructive" @click="remove">
          {{ $t('admin.chat.disconnect') }}
        </AlertDialogAction>
      </AlertDialogFooter>
    </AlertDialogContent>
  </AlertDialog>
</template>

<script setup>
import { computed, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { Button } from '@shared-ui/components/ui/button'
import { Checkbox } from '@shared-ui/components/ui/checkbox'
import { Switch } from '@shared-ui/components/ui/switch'
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
import { handleHTTPError } from '@shared-ui/utils/http.js'
import api from '@/api'
import { useEmitter } from '@main/composables/useEmitter'
import { EMITTER_EVENTS } from '@main/constants/emitterEvents.js'

const props = defineProps({
  connection: { type: Object, required: true },
  events: { type: Array, required: true }
})
const emit = defineEmits(['changed'])

const { t } = useI18n()
const emitter = useEmitter()
const selected = ref([...(props.connection.events || [])])
const isActive = ref(props.connection.is_active)
const saving = ref(false)
const testing = ref(false)
const confirmOpen = ref(false)

const channelLabel = computed(() => {
  const name = props.connection.channel_name || props.connection.channel_id
  return props.connection.provider === 'slack' && !name.startsWith('#') ? `#${name}` : name
})

const toggle = (event, checked) => {
  selected.value = checked
    ? [...selected.value, event]
    : selected.value.filter((e) => e !== event)
}

const toast = (description, variant = 'success') =>
  emitter.emit(EMITTER_EVENTS.SHOW_TOAST, { variant, description })

const save = async () => {
  saving.value = true
  try {
    await api.updateChatIntegration(props.connection.id, {
      events: selected.value,
      is_active: isActive.value
    })
    toast(t('globals.messages.savedSuccessfully'))
    emit('changed')
  } catch (error) {
    toast(handleHTTPError(error).message, 'destructive')
  } finally {
    saving.value = false
  }
}

const test = async () => {
  testing.value = true
  try {
    await api.testChatIntegration(props.connection.id)
    toast(t('admin.chat.testSent', { channel: channelLabel.value }))
  } catch (error) {
    toast(handleHTTPError(error).message, 'destructive')
  } finally {
    testing.value = false
  }
}

const remove = async () => {
  try {
    await api.deleteChatIntegration(props.connection.id)
    emit('changed')
  } catch (error) {
    toast(handleHTTPError(error).message, 'destructive')
  }
}
</script>
