import { ref, nextTick } from 'vue/dist/vue.esm-bundler.js'
import { api } from '../api.js'

export function useGlobalSearch() {
  const globalQuery = ref('')
  const globalResults = ref([])
  const globalBusy = ref(false)
  const globalError = ref('')
  const globalHasMore = ref(false)
  const globalCursor = ref({ time: '', id: '' })
  const globalStarted = ref(false)
  const showGlobalSearch = ref(false)
  const globalInput = ref(null)
  function resetGlobalSearch() {
    globalResults.value = []
    globalCursor.value = { time: '', id: '' }
    globalHasMore.value = false
    globalStarted.value = false
    globalError.value = ''
  }
  function closeGlobalSearch() {
    resetGlobalSearch()
    globalQuery.value = ''
    showGlobalSearch.value = false
  }
  async function openGlobalSearch() {
    showGlobalSearch.value = true
    await nextTick()
    globalInput.value?.focus()
  }
  // Global search spans every conversation the caller belongs to, so each hit
  // carries its own conversation context instead of using the active one.
  async function searchAll(more = false) {
    const query = globalQuery.value.trim()
    if (!query || (more && (globalBusy.value || !globalHasMore.value))) return
    if ([...query].length > 100) { globalError.value = '关键词最多 100 个字符'; return }
    globalBusy.value = true
    globalError.value = ''
    globalStarted.value = true
    try {
      const params = new URLSearchParams({ q: query, limit: '20' })
      if (more && globalCursor.value.time && globalCursor.value.id) {
        params.set('before_time', globalCursor.value.time)
        params.set('before_id', globalCursor.value.id)
      }
      const page = await api(`/messages/search?${params}`)
      globalResults.value = more ? [...globalResults.value, ...page.results] : page.results
      globalHasMore.value = page.has_more
      globalCursor.value = { time: page.next_before_time, id: page.next_before_id }
    } catch (cause) { globalError.value = cause.message || '搜索失败，请重试' }
    finally { globalBusy.value = false }
  }

  return {
    globalQuery, globalResults, globalBusy, globalError, globalStarted, globalHasMore, globalCursor,
    showGlobalSearch, globalInput, resetGlobalSearch, closeGlobalSearch, openGlobalSearch, searchAll,
  }
}
