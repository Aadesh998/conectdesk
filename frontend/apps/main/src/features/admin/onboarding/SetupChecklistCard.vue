<template>
  <router-link
    v-if="visible"
    :to="{ name: 'admin-setup' }"
    class="mx-2 mb-2 block rounded-lg border bg-card p-3 space-y-2 hover:bg-accent transition-colors focus:outline-none focus-visible:ring-2 focus-visible:ring-ring"
  >
    <div class="flex items-center gap-2 text-sm font-medium">
      <Rocket class="size-4 text-primary" />
      <span class="truncate">{{ $t('onboarding.title') }}</span>
    </div>
    <p class="text-xs text-muted-foreground">
      {{ $t('onboarding.progress', { done: doneCount, total: onboardingSteps.length }) }}
    </p>
    <div class="h-1 rounded-full bg-muted overflow-hidden">
      <div class="h-full rounded-full bg-primary" :style="{ width: (doneCount / onboardingSteps.length) * 100 + '%' }" />
    </div>
  </router-link>
</template>

<script setup>
import { ref, computed, watch } from 'vue'
import { Rocket } from 'lucide-vue-next'
import api from '@main/api'
import { useUserStore } from '@main/stores/user'
import { onboardingSteps } from './steps.js'

// Shown to admins until the workspace is set up or an admin hides it.
const userStore = useUserStore()
const data = ref(null)
const doneCount = computed(() => onboardingSteps.filter((s) => data.value?.steps[s.key]).length)
const visible = computed(() => data.value && !data.value.dismissed && doneCount.value < onboardingSteps.length)

// The user can load after the sidebar mounts.
watch(
  () => userStore.can('general_settings:manage'),
  async (isAdmin) => {
    if (!isAdmin || data.value) return
    try {
      data.value = (await api.getOnboarding()).data.data
    } catch {
      // The card is a shortcut; without it the checklist is still in Admin.
    }
  },
  { immediate: true }
)
</script>
