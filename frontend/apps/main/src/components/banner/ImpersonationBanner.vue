<template>
  <div
    v-if="admin"
    role="status"
    class="flex flex-wrap items-center gap-x-3 gap-y-1 border-b bg-warning px-4 py-2 text-sm text-warning-foreground"
  >
    <ShieldAlert class="size-4 shrink-0" />
    <span class="min-w-0 flex-1">{{ $t('impersonation.banner', { admin, name: userStore.getFullName }) }}</span>
    <a href="/logout" class="font-semibold underline underline-offset-2">{{ $t('impersonation.end') }}</a>
  </div>
</template>

<script setup>
import { computed } from 'vue'
import { ShieldAlert } from 'lucide-vue-next'
import { useUserStore } from '@/stores/user'

// A platform admin's "log in as" session: say so on every page.
const userStore = useUserStore()
const admin = computed(() => userStore.user.impersonated_by)
</script>
