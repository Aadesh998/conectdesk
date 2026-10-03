<template>
  <div class="grid grid-cols-[5rem_1fr] gap-x-3 gap-y-1 rounded-md bg-muted p-3 font-mono text-xs">
    <span class="text-muted-foreground">{{ $t('globals.terms.type') }}</span>
    <span>{{ type }}</span>
    <span class="text-muted-foreground">{{ $t('globals.terms.name') }}</span>
    <button type="button" class="break-all text-left hover:underline" @click="copy(name)">{{ name }}</button>
    <span class="text-muted-foreground">{{ $t('globals.terms.value') }}</span>
    <button type="button" class="break-all text-left hover:underline" @click="copy(value)">{{ value }}</button>
  </div>
</template>

<script setup>
import { useI18n } from 'vue-i18n'
import { EMITTER_EVENTS } from '@main/constants/emitterEvents.js'
import { useEmitter } from '@main/composables/useEmitter'

defineProps({
  type: { type: String, required: true },
  name: { type: String, required: true },
  value: { type: String, required: true }
})

const { t } = useI18n()
const emitter = useEmitter()

const copy = async (text) => {
  try {
    await navigator.clipboard.writeText(text)
    emitter.emit(EMITTER_EVENTS.SHOW_TOAST, { description: t('globals.messages.copied') })
  } catch {
    // Clipboard access denied; the text stays selectable.
  }
}
</script>
