import { ref, watch, onScopeDispose } from 'vue/dist/vue.esm-bundler.js'
import { mediaPreview } from './media.js'

export function messageNotificationText(message) {
  const text = message?.content?.trim()
  return (text || mediaPreview(message) || '新消息').slice(0, 160)
}

// Only newly synchronized messages enter here; baseline sequences suppress historical replay.
export function useMessageNotifications({ me, conversations, title, onOpen, showError, clock = () => Date.now() }) {
  const notices = ref([])
  const notificationPermission = ref(globalThis.Notification?.permission || 'unsupported')
  const watermarks = new Map(), timers = new Map(), systemNotices = new Set(), recentOnly = new Set()
  let primed = false, startedAt = clock(), serial = 0
  function dismissNotice(id) {
    clearTimeout(timers.get(id)); timers.delete(id)
    notices.value = notices.value.filter(item => item.id !== id)
  }
  function reset() {
    for (const timer of timers.values()) clearTimeout(timer)
    for (const item of systemNotices) item.close?.()
    systemNotices.clear(); timers.clear(); watermarks.clear(); recentOnly.clear(); notices.value = []
    primed = false; startedAt = clock()
  }
  watch(() => me.value?.id, reset, { flush: 'sync' })
  function prime(list) {
    if (primed) return
    for (const item of list) watermarks.set(item.id, Number(item.last_seq) || 0)
    primed = true
  }
  function openNotice(notice) {
    const conversation = conversations.value.find(item => item.id === notice.conversationID)
    dismissNotice(notice.id)
    if (conversation && me.value) { window.focus?.(); Promise.resolve(onOpen(notice.conversationID)).catch(showError) }
  }
  function receive(conversationID, messages) {
    if (!primed || !me.value) return
    const conversation = conversations.value.find(item => item.id === conversationID)
    if (!conversation) return
    const known = watermarks.has(conversationID)
    if (!known) recentOnly.add(conversationID)
    let watermark = watermarks.get(conversationID) || 0
    const incoming = []
    for (const message of [...messages].sort((a, b) => a.seq - b.seq)) {
      if (message.seq <= watermark) continue
      watermark = message.seq
      if (recentOnly.has(conversationID) && (!message.created_at || Date.parse(message.created_at) < startedAt)) continue
      if (message.sender_id === me.value.id || message.recalled_at || message.kind === 'system') continue
      incoming.push(message)
    }
    watermarks.set(conversationID, watermark)
    if (conversation.muted || !incoming.length) return
    const latest = incoming.at(-1)
    const sender = conversation.members?.find(member => member.id === latest.sender_id)
    const senderName = sender?.alias || sender?.name || '用户'
    const previous = notices.value.find(item => item.conversationID === conversationID)
    const notice = {
      id: previous?.id || ++serial, conversationID, title: title(conversation),
      text: `${senderName}：${messageNotificationText(latest)}`, count: (previous?.count || 0) + incoming.length,
    }
    if (previous) dismissNotice(previous.id)
    notices.value = [...notices.value, notice]
    while (notices.value.length > 3) dismissNotice(notices.value[0].id)
    timers.set(notice.id, setTimeout(() => dismissNotice(notice.id), 7000))
    const NativeNotification = globalThis.Notification
    if (NativeNotification?.permission === 'granted' && (document.visibilityState === 'hidden' || !document.hasFocus?.())) {
      try {
        const popup = new NativeNotification(notice.title, { body: notice.text, tag: `chat:${me.value.id}:${conversationID}`, icon: '/default-avatar.svg' })
        systemNotices.add(popup)
        popup.onclick = () => { popup.close?.(); openNotice(notice) }
        popup.onclose = () => systemNotices.delete(popup)
      } catch { /* In-app notices remain available when the platform has no native support. */ }
    }
  }
  function reconcilePreferences() {
    for (const notice of [...notices.value]) {
      const item = conversations.value.find(conversation => conversation.id === notice.conversationID)
      if (!item || item.muted) dismissNotice(notice.id)
    }
  }
  watch(conversations, reconcilePreferences, { deep: true })
  async function enableSystemNotifications() {
    const NativeNotification = globalThis.Notification
    if (!NativeNotification?.requestPermission) { showError(new Error('当前环境不支持系统通知，新消息仍会在应用内提示')); return }
    try {
      notificationPermission.value = await NativeNotification.requestPermission()
      if (notificationPermission.value === 'denied') showError(new Error('系统通知已被浏览器阻止，可在网站权限设置中开启；应用内提醒仍可使用'))
    } catch { showError(new Error('无法开启系统通知，新消息仍会在应用内提示')) }
  }
  onScopeDispose(reset)
  return { notices, notificationPermission, prime, receive, openNotice, dismissNotice, enableSystemNotifications }
}
