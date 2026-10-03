<template>
  <div class="box p-6 space-y-4 max-w-2xl">
    <div class="flex items-center gap-3">
      <component :is="channelIcon(channel)" class="size-6" />
      <h3 class="font-semibold text-lg">{{ title }}</h3>
    </div>

    <p class="text-sm text-muted-foreground">{{ $t(`admin.inbox.meta.help.${channel}`) }}</p>
    <p class="text-xs text-muted-foreground">{{ $t('admin.inbox.meta.window') }}</p>

    <p v-if="!available" class="text-sm text-destructive">
      {{ $t('admin.inbox.meta.unavailable', { channel: title }) }}
    </p>

    <MetaConnectButton :channel="channel" />
  </div>
</template>

<script setup>
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import { useAppSettingsStore } from '@/stores/appSettings'
import { channelIcon } from '@/features/conversation/channelIcon.js'
import MetaConnectButton from './MetaConnectButton.vue'

const props = defineProps({
  // whatsapp, messenger or instagram.
  channel: {
    type: String,
    required: true
  }
})

const { t } = useI18n()
const appSettingsStore = useAppSettingsStore()

const title = computed(() => t(`globals.terms.${props.channel}`))
const available = computed(() =>
  (appSettingsStore.public_config['app.meta_products'] || []).includes(props.channel)
)
</script>
