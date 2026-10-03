<template>
  <!-- The CSV a contact import takes, shown in the import dialog. -->
  <div class="space-y-3 text-xs">
    <div class="bg-muted p-3 rounded-md font-mono overflow-x-auto leading-relaxed whitespace-pre">{{ sample }}</div>

    <table class="w-full">
      <thead class="text-left text-muted-foreground">
        <tr>
          <th class="py-1 pr-2 font-medium">{{ $t('contact.import.column') }}</th>
          <th class="py-1 font-medium">{{ $t('contact.import.meaning') }}</th>
        </tr>
      </thead>
      <tbody>
        <tr v-for="col in columns" :key="col.name" class="border-t align-top">
          <td class="py-1 pr-2 font-mono whitespace-nowrap">
            {{ col.name }}
            <span v-if="col.required" class="text-destructive">*</span>
          </td>
          <td class="py-1 text-muted-foreground">{{ $t(col.hint) }}</td>
        </tr>
      </tbody>
    </table>

    <p class="text-muted-foreground">{{ $t('contact.import.notes') }}</p>

    <button type="button" class="inline-flex items-center gap-1 underline" @click="download">
      <Download :size="12" />
      {{ $t('contact.import.downloadSample') }}
    </button>
  </div>
</template>

<script setup>
import { Download } from 'lucide-vue-next'

const sample = `first_name,last_name,phone_number,country_code,email
Ravi,Kumar,+91 98765 43210,,ravi@example.com
Priya,Shah,9876501234,IN,
Amit,,98765 00000,91,`

const columns = [
  { name: 'first_name', required: true, hint: 'contact.import.col.firstName' },
  { name: 'last_name', hint: 'contact.import.col.lastName' },
  { name: 'phone_number', hint: 'contact.import.col.phone' },
  { name: 'country_code', hint: 'contact.import.col.country' },
  { name: 'email', hint: 'contact.import.col.email' },
  { name: 'external_user_id', hint: 'contact.import.col.externalId' }
]

const download = () => {
  const url = URL.createObjectURL(new Blob([sample + '\n'], { type: 'text/csv' }))
  const a = document.createElement('a')
  a.href = url
  a.download = 'contacts-sample.csv'
  a.click()
  URL.revokeObjectURL(url)
}
</script>
