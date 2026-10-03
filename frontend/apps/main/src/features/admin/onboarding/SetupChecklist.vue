<template>
  <AdminSplitLayout>
    <template #content>
      <div class="space-y-6" :class="{ 'opacity-50 transition-opacity duration-300': !data }">
        <div class="space-y-2">
          <p class="text-sm text-muted-foreground">{{ $t('onboarding.description') }}</p>
          <p class="text-sm font-medium">{{ $t('onboarding.progress', { done: doneCount, total: onboardingSteps.length }) }}</p>
          <div class="h-1.5 max-w-sm rounded-full bg-muted overflow-hidden">
            <div class="h-full rounded-full bg-primary transition-all" :style="{ width: (doneCount / onboardingSteps.length) * 100 + '%' }" />
          </div>
        </div>

        <p v-if="data && doneCount === onboardingSteps.length" class="text-sm text-success">{{ $t('onboarding.allDone') }}</p>

        <ol v-if="data" class="space-y-3">
          <li
            v-for="s in onboardingSteps"
            :key="s.key"
            class="flex items-center gap-4 rounded-lg border p-4"
            :class="{ 'bg-muted/40': data.steps[s.key] }"
          >
            <CircleCheck v-if="data.steps[s.key]" class="size-5 shrink-0 text-success" :aria-label="$t('onboarding.done')" />
            <Circle v-else class="size-5 shrink-0 text-muted-foreground" />
            <div class="min-w-0 flex-1 space-y-0.5">
              <p class="text-sm font-medium" :class="{ 'line-through text-muted-foreground': data.steps[s.key] }">
                {{ $t('onboarding.steps.' + s.key) }}
              </p>
              <p class="text-xs text-muted-foreground">{{ $t('onboarding.steps.' + s.key + 'Description') }}</p>
            </div>
            <router-link v-if="!data.steps[s.key]" :to="s.to">
              <Button size="sm" variant="outline">{{ $t('onboarding.start') }}</Button>
            </router-link>
          </li>
        </ol>

        <Button v-if="data && !data.dismissed" variant="ghost" size="sm" :isLoading="hiding" @click="hide">
          {{ $t('onboarding.hide') }}
        </Button>
      </div>
    </template>
  </AdminSplitLayout>
</template>

<script setup>
import { ref, computed, onMounted } from 'vue'
import { Circle, CircleCheck } from 'lucide-vue-next'
import api from '@main/api'
import AdminSplitLayout from '@main/layouts/admin/AdminSplitLayout.vue'
import { EMITTER_EVENTS } from '@main/constants/emitterEvents.js'
import { useEmitter } from '@main/composables/useEmitter'
import { handleHTTPError } from '@shared-ui/utils/http.js'
import { Button } from '@shared-ui/components/ui/button'
import { onboardingSteps } from './steps.js'

const emitter = useEmitter()
const data = ref(null)
const hiding = ref(false)
const doneCount = computed(() => onboardingSteps.filter((s) => data.value?.steps[s.key]).length)

const toastError = (error) =>
  emitter.emit(EMITTER_EVENTS.SHOW_TOAST, { variant: 'destructive', description: handleHTTPError(error).message })

const hide = async () => {
  hiding.value = true
  try {
    await api.dismissOnboarding()
    data.value.dismissed = true
  } catch (error) {
    toastError(error)
  } finally {
    hiding.value = false
  }
}

onMounted(async () => {
  try {
    data.value = (await api.getOnboarding()).data.data
  } catch (error) {
    toastError(error)
  }
})
</script>
