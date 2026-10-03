<template>
  <!-- Buttons shown under a Telegram reply: a quick reply the customer taps to
       answer with its label, or a link. -->
  <div class="mx-2 mt-2 flex flex-wrap items-center gap-1.5 text-sm">
    <span
      v-for="(b, i) in model"
      :key="i"
      class="inline-flex items-center gap-1 rounded-full border px-2 py-0.5 text-xs"
    >
      <Link2 v-if="b.url" :size="12" class="text-muted-foreground" />
      {{ b.text }}
      <button type="button" class="text-muted-foreground hover:text-foreground" @click="remove(i)">✕</button>
    </span>
    <template v-if="adding">
      <Input v-model="text" class="h-7 w-40 text-xs" maxlength="60" :placeholder="$t('telegram.button.label')" @keydown.enter.prevent="add" />
      <Input v-model="url" class="h-7 w-52 text-xs" :placeholder="$t('telegram.button.url')" @keydown.enter.prevent="add" />
      <Button type="button" size="sm" class="h-7" :disabled="!text.trim()" @click="add">{{ $t('globals.messages.add') }}</Button>
    </template>
    <Button v-else-if="model.length < 10" type="button" variant="ghost" size="sm" class="h-7 text-xs" @click="adding = true">
      + {{ $t('telegram.addButton') }}
    </Button>
  </div>
</template>

<script setup>
import { ref } from 'vue'
import { Link2 } from 'lucide-vue-next'
import { Button } from '@shared-ui/components/ui/button'
import { Input } from '@shared-ui/components/ui/input'

const model = defineModel({ type: Array, default: () => [] })
const adding = ref(false)
const text = ref('')
const url = ref('')

const add = () => {
  if (!text.value.trim()) return
  const b = { text: text.value.trim() }
  if (url.value.trim()) b.url = url.value.trim()
  model.value = [...model.value, b]
  text.value = ''
  url.value = ''
  adding.value = false
}

const remove = (i) => {
  model.value = model.value.filter((_, j) => j !== i)
}
</script>
