import { ref, computed, nextTick } from 'vue/dist/vue.esm-bundler.js'
import { api } from '../api.js'
import { historySearchParams } from '../history-filters.js'

export function useHistorySearch({ active, current, onReset }) {
  const showSearch = ref(false)
  const searchInput = ref(null)
  const searchQuery = ref('')
  const searchedQuery = ref('')
  const searchSender = ref('')
  const historyMemberQuery = ref('')
  const historyMemberOptions = computed(() => {
    const query = historyMemberQuery.value.trim().toLocaleLowerCase()
    return (current.value?.members || []).filter(person => person.id === searchSender.value
      || [person.alias, person.name, person.email].some(value => String(value || '').toLocaleLowerCase().includes(query)))
  })
  const searchType = ref('all')
  const searchStart = ref('')
  const searchEnd = ref('')
  let searchedParams = ''
  const searchResults = ref([])
  const searchBusy = ref(false)
  const searchError = ref('')
  const searchHasMore = ref(false)
  const searchBefore = ref(0)
  const searchStarted = ref(false)
  let searchController
  let searchVersion = 0
  function resetSearch() {
    searchVersion++
    onReset()
    searchController?.abort()
    searchBusy.value = false
    searchResults.value = []
    searchedQuery.value = ''
    searchedParams = ''
    searchHasMore.value = false
    searchBefore.value = 0
    searchStarted.value = false
    searchError.value = ''
  }
  function closeSearch() {
    resetSearch()
    showSearch.value = false
    searchQuery.value = ''
    searchSender.value = ''
    historyMemberQuery.value = ''
    searchType.value = 'all'
    searchStart.value = ''
    searchEnd.value = ''
  }
  function clearSearchFilters() {
    searchQuery.value = ''
    searchSender.value = ''
    historyMemberQuery.value = ''
    searchType.value = 'all'
    searchStart.value = ''
    searchEnd.value = ''
    resetSearch()
    searchMessages()
  }
  async function openSearch() {
    showSearch.value = true
    await nextTick()
    searchInput.value?.focus()
    if (!searchStarted.value) searchMessages()
  }
  function changeSearch() {
    resetSearch()
  }
  async function searchMessages(more = false) {
    const query = more ? searchedQuery.value : searchQuery.value.trim()
    if (!active.value || (more && (searchBusy.value || !searchHasMore.value))) return
    if (!more) resetSearch()
    let params
    try {
      params = more ? new URLSearchParams(searchedParams) : historySearchParams({
        query, sender: searchSender.value, type: searchType.value, start: searchStart.value, end: searchEnd.value,
      })
    } catch (cause) { searchError.value = cause.message; return }
    const id = active.value
    const version = ++searchVersion
    searchController?.abort()
    const controller = new AbortController()
    searchController = controller
    searchBusy.value = true
    searchStarted.value = true
    searchError.value = ''
    searchedQuery.value = query
    if (!more) searchedParams = params.toString()
    if (more) params.set('before_seq', String(searchBefore.value))
    try {
      const page = await api(`/conversations/${id}/messages/search?${params}`, { signal: controller.signal })
      if (version !== searchVersion || id !== active.value) return
      searchResults.value = more ? [...searchResults.value, ...page.messages] : page.messages
      searchBefore.value = page.next_before_seq
      searchHasMore.value = page.has_more
    } catch (cause) {
      if (version === searchVersion && !controller.signal.aborted) searchError.value = '搜索失败，请重试'
    } finally {
      if (version === searchVersion) searchBusy.value = false
    }
  }

  return {
    showSearch, searchInput, searchQuery, searchedQuery, searchSender, historyMemberQuery,
    historyMemberOptions, searchType, searchStart, searchEnd, searchResults, searchBusy, searchError,
    searchHasMore, searchBefore, searchStarted, resetSearch, closeSearch, clearSearchFilters, openSearch,
    changeSearch, searchMessages,
  }
}
