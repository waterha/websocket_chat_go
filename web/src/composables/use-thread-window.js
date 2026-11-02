import { ref, nextTick } from 'vue/dist/vue.esm-bundler.js'
import { api } from '../api.js'
import { threadRootID, threadHasMore } from '../thread.js'

export function useThreadWindow({ active, syncConversation, showError, getEpoch }) {
  // The thread window is a surface separate from the chat list: it shows one
  // root message and every reply to it, and replies sent here join that window.
  const threadOpen = ref(false)
  const threadRoot = ref(null)
  const threadReplies = ref([])
  const threadHasMorePages = ref(false)
  const threadBusy = ref(false)
  const threadError = ref('')
  const threadReplyCount = ref(0)
  const threadStream = ref(null)
  function closeThread() {
    threadOpen.value = false
    threadRoot.value = null
    threadReplies.value = []
    threadError.value = ''
    threadHasMorePages.value = false
    threadReplyCount.value = 0
    // A draft typed in the window is not carried over to the next one.
    threadDraft.value = ''
  }
  async function openThread(item) {
    if (!item) return
    const id = active.value
    const rootID = threadRootID(item)
    const version = getEpoch()
    threadOpen.value = true
    threadRoot.value = item
    threadReplies.value = []
    threadError.value = ''
    threadHasMorePages.value = false
    threadBusy.value = true
    try {
      const page = await api(`/conversations/${id}/messages/${rootID}/thread?limit=100`)
      if (version !== getEpoch()) return
      // The server resolves a reply to its window root, so the header shows the
      // root even when the user opened the window from a reply.
      threadRoot.value = page.root
      threadReplies.value = page.replies || []
      threadReplyCount.value = page.reply_count || 0
      threadHasMorePages.value = threadHasMore(page)
      await nextTick()
      threadStream.value?.scrollTo({ top: threadStream.value.scrollHeight })
    } catch (cause) {
      if (version === getEpoch()) threadError.value = cause.message || '打开对话窗口失败'
    } finally { if (version === getEpoch()) threadBusy.value = false }
  }
  async function loadMoreThread() {
    if (threadBusy.value || !threadHasMorePages.value || !threadRoot.value) return
    const id = active.value
    const version = getEpoch()
    const last = threadReplies.value.at(-1)
    threadBusy.value = true
    try {
      const page = await api(`/conversations/${id}/messages/${threadRoot.value.id}/thread?limit=100&after_seq=${last?.seq || 0}`)
      if (version !== getEpoch()) return
      threadReplies.value = [...threadReplies.value, ...(page.replies || [])]
      threadHasMorePages.value = threadHasMore(page)
      threadReplyCount.value = page.reply_count || threadReplyCount.value
    } catch (cause) { if (version === getEpoch()) threadError.value = cause.message || '加载失败' }
    finally { if (version === getEpoch()) threadBusy.value = false }
  }
  // The window has its own composer: a reply written here must be sent into the
  // window without disturbing whatever the user had typed in the main chat.
  const threadDraft = ref('')
  const threadSending = ref(false)
  const threadDraftArea = ref(null)
  async function sendInThread() {
    const content = threadDraft.value.trim()
    const root = threadRoot.value
    const id = active.value
    const version = getEpoch()
    if (!content || !root || !id || threadSending.value) return
    threadSending.value = true
    try {
      await api(`/conversations/${id}/messages`, {
        method: 'POST',
        body: JSON.stringify({ client_msg_id: crypto.randomUUID(), content, media_id: '', reply_to_id: root.id, mentions: [] }),
      })
      if (version !== getEpoch()) return
      threadDraft.value = ''
      await refreshThread()
      await syncConversation(id)
    } catch (cause) { if (version === getEpoch()) showError(cause) }
    finally { if (version === getEpoch()) threadSending.value = false }
  }
  // Reloads the window after a change so the reply appears with its true count.
  async function refreshThread() {
    const root = threadRoot.value
    const id = active.value
    if (!root || !id) return
    const page = await api(`/conversations/${id}/messages/${root.id}/thread?limit=100`)
    threadRoot.value = page.root
    threadReplies.value = page.replies || []
    threadReplyCount.value = page.reply_count || 0
    threadHasMorePages.value = threadHasMore(page)
    await nextTick()
    threadStream.value?.scrollTo({ top: threadStream.value.scrollHeight })
  }
  function onThreadEnter(event) {
    if (event.isComposing) return
    event.preventDefault()
    sendInThread()
  }

  return {
    threadOpen, threadRoot, threadReplies, threadHasMorePages, threadBusy, threadError, threadReplyCount,
    threadStream, threadDraft, threadSending, threadDraftArea, closeThread, openThread, loadMoreThread,
    sendInThread, refreshThread, onThreadEnter,
  }
}
