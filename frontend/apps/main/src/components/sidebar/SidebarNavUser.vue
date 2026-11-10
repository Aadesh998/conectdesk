<template>
  <DropdownMenu>
    <DropdownMenuTrigger as-child>
      <SidebarMenuButton
        size="default"
        class="p-0 !overflow-visible"
      >
        <div class="relative">
          <Avatar class="h-8 w-8 rounded-md">
            <AvatarImage :src="userStore.avatar" alt="U" class="rounded-md" />
            <AvatarFallback class="rounded-md">
              {{ userStore.getInitials }}
            </AvatarFallback>
          </Avatar>
          <StatusDot
            :status="userStore.user.availability_status"
            size="sm"
            class="absolute bottom-0 right-0 border border-background"
          />
        </div>
        <div class="grid flex-1 text-left text-sm leading-tight">
          <span class="truncate font-semibold">{{ userStore.getFullName }}</span>
          <span class="truncate text-xs">{{ userStore.email }}</span>
        </div>
        <ChevronsUpDown class="ml-auto size-4" />
      </SidebarMenuButton>
    </DropdownMenuTrigger>
    <DropdownMenuContent
      class="min-w-56"
      :side="isMobile ? 'bottom' : 'right'"
      :align="isMobile ? 'start' : 'end'"
      :side-offset="8"
      :align-offset="isMobile ? 0 : 40"
    >
      <DropdownMenuLabel class="font-normal space-y-2 px-2">
        <!-- User header -->
        <div class="flex items-center gap-2 py-1.5 text-left text-sm">
          <Avatar class="h-8 w-8 rounded-md">
            <AvatarImage :src="userStore.avatar" alt="U" />
            <AvatarFallback class="rounded-md">
              {{ userStore.getInitials }}
            </AvatarFallback>
          </Avatar>
          <div class="flex-1 flex flex-col leading-tight">
            <span class="truncate font-semibold">{{ userStore.getFullName }}</span>
            <span class="truncate text-xs text-muted-foreground">{{ userStore.email }}</span>
          </div>
        </div>

        <div class="space-y-2">
          <!-- Theme: light, dark or follow the system -->
          <div class="flex items-center justify-between gap-2 text-sm">
            <span class="text-muted-foreground">{{ t('navigation.theme') }}</span>
            <div
              role="radiogroup"
              :aria-label="t('navigation.theme')"
              class="inline-flex items-center gap-0.5 rounded-md border p-0.5"
            >
              <button
                v-for="opt in themeOptions"
                :key="opt.value"
                type="button"
                role="radio"
                :aria-checked="store === opt.value"
                :aria-label="opt.label"
                :title="opt.label"
                class="inline-flex size-6 items-center justify-center rounded-sm text-muted-foreground transition-colors hover:text-foreground focus-visible:outline-none focus-visible:ring-1 focus-visible:ring-ring"
                :class="store === opt.value && 'bg-accent text-foreground'"
                @click.stop="store = opt.value"
              >
                <component :is="opt.icon" class="size-3.5" />
              </button>
            </div>
          </div>

          <div class="border-t border-border pt-3 space-y-3">
            <!-- Away toggle -->
            <div class="flex items-center justify-between text-sm">
              <span class="text-muted-foreground">{{ t('navigation.away') }}</span>
              <Switch
                :checked="
                  ['away_manual', 'away_and_reassigning'].includes(
                    userStore.user.availability_status
                  )
                "
                @update:checked="
                  (val) => userStore.updateUserAvailability(val ? 'away_manual' : 'online')
                "
              />
            </div>
            <!-- Reassign toggle -->
            <div class="flex items-center justify-between text-sm">
              <span class="text-muted-foreground">{{ t('navigation.reassignReplies') }}</span>
              <Switch
                :checked="userStore.user.availability_status === 'away_and_reassigning'"
                @update:checked="
                  (val) =>
                    userStore.updateUserAvailability(val ? 'away_and_reassigning' : 'away_manual')
                "
              />
            </div>
          </div>
        </div>
      </DropdownMenuLabel>
      <DropdownMenuSeparator />
      <DropdownMenuGroup>
        <DropdownMenuItem @click.prevent="router.push({ name: 'account' })">
          <CircleUserRound size="18" class="mr-2" />
          {{ t('globals.terms.account') }}
        </DropdownMenuItem>
      </DropdownMenuGroup>
      <DropdownMenuSeparator />
      <DropdownMenuGroup>
        <DropdownMenuItem @click="showShortcuts = true">
          <Keyboard size="18" class="mr-2" />
          {{ t('navigation.keyboardShortcuts') }}
        </DropdownMenuItem>
      </DropdownMenuGroup>
      <DropdownMenuSeparator />
      <DropdownMenuItem @click="logout">
        <LogOut size="18" class="mr-2" />
        {{ t('navigation.logout') }}
      </DropdownMenuItem>
    </DropdownMenuContent>
  </DropdownMenu>

  <KeyboardShortcutsDialog v-model:open="showShortcuts" />
</template>

<script setup>
import { useI18n } from 'vue-i18n'
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuGroup,
  DropdownMenuItem,
  DropdownMenuLabel,
  DropdownMenuSeparator,
  DropdownMenuTrigger
} from '@shared-ui/components/ui/dropdown-menu'
import { SidebarMenuButton } from '@shared-ui/components/ui/sidebar'
import { Avatar, AvatarFallback, AvatarImage } from '@shared-ui/components/ui/avatar'
import StatusDot from '@shared-ui/components/StatusDot.vue'
import { Switch } from '@shared-ui/components/ui/switch'
import {
  ChevronsUpDown,
  CircleUserRound,
  Keyboard,
  LogOut,
  Monitor,
  Moon,
  Sun
} from 'lucide-vue-next'
import { useUserStore } from '@main/stores/user'
import { useIsMobile } from '@shared-ui/composables'
import { useRouter } from 'vue-router'
import KeyboardShortcutsDialog from '@main/components/KeyboardShortcutsDialog.vue'

import { useColorMode } from '@vueuse/core'
import { computed, ref } from 'vue'

const isMobile = useIsMobile()
// store is the preference (light, dark or auto); the html class follows it.
const { store } = useColorMode()
const userStore = useUserStore()
const router = useRouter()
const { t } = useI18n()
const showShortcuts = ref(false)
const themeOptions = computed(() => [
  { value: 'light', label: t('navigation.theme.light'), icon: Sun },
  { value: 'dark', label: t('navigation.theme.dark'), icon: Moon },
  { value: 'auto', label: t('navigation.theme.system'), icon: Monitor }
])

const logout = () => {
  window.location.href = '/logout'
}
</script>
