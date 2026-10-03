<template>
  <AdminSplitLayout>
    <template #content>
      <div class="space-y-10">
        <LeadStagesSection />
        <DailyReportSection />
        <template v-if="userStore.can('ai:manage')">
          <AIFeaturesSection />
          <AdmissionsSetupSection />
        </template>
      </div>
    </template>
  </AdminSplitLayout>
</template>

<script setup>
import AdminSplitLayout from '@main/layouts/admin/AdminSplitLayout.vue'
import LeadStagesSection from './LeadStagesSection.vue'
import DailyReportSection from './DailyReportSection.vue'
import AIFeaturesSection from './AIFeaturesSection.vue'
import AdmissionsSetupSection from './AdmissionsSetupSection.vue'
import { useUserStore } from '@main/stores/user'

const userStore = useUserStore()
</script>
