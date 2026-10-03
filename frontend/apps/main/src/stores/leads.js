import { ref } from 'vue'
import { defineStore } from 'pinia'
import api from '../api'

// The workspace's lead stages, and the last stage move the server told us
// about, which the leads board and contact panels watch.
export const useLeadStore = defineStore('leads', () => {
  const stages = ref([])
  const lastChange = ref(null)
  let loaded = false

  const fetchStages = async (force = false) => {
    if (loaded && !force) return stages.value
    try {
      const resp = await api.getLeadStages()
      stages.value = resp.data.data || []
      loaded = true
    } catch {
      // Keep what we had.
    }
    return stages.value
  }

  const stageOf = (key) => stages.value.find((s) => s.key === key)

  const changed = (data) => {
    lastChange.value = { ...(data || {}), at: Date.now() }
  }

  return { stages, lastChange, fetchStages, stageOf, changed }
})
