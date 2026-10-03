<template>
  <div class="box p-4 space-y-3">
    <div class="flex items-center justify-between gap-2">
      <h3 class="font-semibold">{{ $t('whatsapp.health') }}</h3>
      <Button type="button" variant="outline" size="sm" :is-loading="loading" @click="load">
        {{ $t('whatsapp.refreshTemplates') }}
      </Button>
    </div>

    <!-- The last thing Meta reported, until the next report. -->
    <div v-if="alert" class="rounded-md border px-3 py-2 text-sm" :class="alertClass">
      <p>{{ alert.detail }}</p>
      <p class="text-xs opacity-75">{{ formatTime(alert.at) }}</p>
    </div>

    <p v-if="error" class="text-sm text-muted-foreground">{{ $t('whatsapp.healthUnavailable', { error }) }}</p>
    <dl v-else-if="health" class="grid grid-cols-[max-content_1fr] gap-x-4 gap-y-1.5 text-sm">
      <dt class="text-muted-foreground">{{ $t('whatsapp.quality') }}</dt>
      <dd class="flex items-center gap-2">
        <span class="inline-block size-2 rounded-full" :class="qualityDot" />
        {{ $t(`whatsapp.quality.${(health.quality_rating || 'UNKNOWN').toLowerCase()}`) }}
      </dd>
      <dt class="text-muted-foreground">{{ $t('whatsapp.messagingLimit') }}</dt>
      <dd>{{ limit }}</dd>
      <dt class="text-muted-foreground">{{ $t('globals.terms.status') }}</dt>
      <dd>{{ health.status }}</dd>
      <dt class="text-muted-foreground">{{ $t('whatsapp.displayName') }}</dt>
      <dd>{{ health.verified_name }} <span class="text-muted-foreground">({{ health.name_status }})</span></dd>
    </dl>
    <p class="text-xs text-muted-foreground">{{ $t('whatsapp.health.description') }}</p>
  </div>
</template>

<script setup>
import { computed, ref, watch } from 'vue'
import { format } from 'date-fns'
import { Button } from '@shared-ui/components/ui/button'
import api from '@/api'

const props = defineProps({
  inboxId: {
    type: [Number, String],
    required: true
  },
  alert: {
    type: Object,
    default: null
  }
})

const health = ref(null)
const limit = ref('')
const error = ref('')
const loading = ref(false)

const qualityDot = computed(
  () =>
    ({ GREEN: 'bg-green-500', YELLOW: 'bg-amber-500', RED: 'bg-destructive' })[health.value?.quality_rating] ||
    'bg-muted-foreground'
)
const alertClass = computed(
  () =>
    ({
      critical: 'border-destructive/50 bg-destructive/10',
      warning: 'border-amber-500/50 bg-amber-500/10'
    })[props.alert?.severity] || 'text-muted-foreground'
)
const formatTime = (ts) => (ts ? format(new Date(ts), 'MMM d, yyyy h:mm a') : '')

const load = async () => {
  loading.value = true
  try {
    const resp = await api.getWhatsAppHealth(props.inboxId)
    health.value = resp.data.data.health || null
    limit.value = resp.data.data.limit || ''
    error.value = resp.data.data.error || ''
  } catch (e) {
    error.value = e?.message || ''
  } finally {
    loading.value = false
  }
}

watch(() => props.inboxId, load, { immediate: true })
</script>
