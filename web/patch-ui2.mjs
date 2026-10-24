// One-off patch: avatars, date dividers, direct info panel, poke.
import fs from 'node:fs'

const path = 'src/main.js'
let src = fs.readFileSync(path, 'utf8').split('\r\n').join('\n')

function patch(old, next) {
  if (!src.includes(old)) throw new Error('MISSING: ' + old.slice(0, 90))
  src = src.replace(old, next)
}

// 1. UserAvatar component + registration
patch(`  components: { Search, X, ArrowRight, RotateCw, Users, Paperclip, Download, Check, DesktopSettings, OrganizationDirectory },`,
`  components: { Search, X, ArrowRight, RotateCw, Users, Paperclip, Download, Check, DesktopSettings, OrganizationDirectory, UserAvatar },`)

patch(`import { chatStorage, createSocket, installDesktopDownloads } from './desktop.js'`,
`import { chatStorage, createSocket, installDesktopDownloads } from './desktop.js'
import { mediaURL } from './media.js'`)

patch(`createApp({`,
`// 头像组件：有头像用图片，否则回退到首字母占位。
const UserAvatar = {
  props: { user: { type: Object, default: null }, name: { type: String, default: '?' }, size: { type: String, default: '' } },
  setup(props) {
    const url = computed(() => (props.user?.avatar ? mediaURL(props.user.avatar) : ''))
    return { url }
  },
  template: '<img v-if="url" :src="url" class="avatar" :class="size" alt="" loading="lazy" /><span v-else class="avatar" :class="size">{{ (name || (chr63) ).slice(0, 1) }}</span>'.replace('chr63', String.fromCharCode(63)),
}

createApp({`)

// 2. helpers: memberByID, headMember, date helpers, poke, direct panel actions
patch(`    const dismissedHere = computed(() => Boolean(current.value?.dismissed))`,
`    const dismissedHere = computed(() => Boolean(current.value?.dismissed))
    const memberByID = id => current.value?.members.find(member => member.id === id) || null
    // 单聊信息面板展示对方资料；群聊沿用右侧成员面板。
    const headMember = computed(() => (current.value?.kind === 'direct' ? memberByID(current.value.members.find(member => member.id !== me.value?.id)?.id) : null))
    const chatInfoOpen = ref(false)
    watch([active, me], () => { chatInfoOpen.value = false })`)

patch(`    // ---- conversation menu and private deletion ----`,
`    // ---- message date dividers ----
    const sameDay = (left, right) => {
      const a = new Date(left)
      const b = new Date(right)
      return a.getFullYear() === b.getFullYear() && a.getMonth() === b.getMonth() && a.getDate() === b.getDate()
    }
    const dayDivider = index => index === 0 || !sameDay(messages.value[index - 1]?.created_at, messages.value[index].created_at)
    const dayLabel = value => {
      const date = new Date(value)
      const now = new Date()
      const startOfToday = new Date(now.getFullYear(), now.getMonth(), now.getDate()).getTime()
      const time = date.getTime()
      if (time >= startOfToday) return '今天'
      if (time >= startOfToday - 86400000) return '昨天'
      const sameYear = date.getFullYear() === now.getFullYear()
      const label = date.getMonth() + 1 + ' 月 ' + date.getDate() + ' 日'
      return sameYear ? label : date.getFullYear() + ' 年 ' + label
    }
    // ---- poke ----
    async function pokeMember(userID) {
      if (!active.value || !userID || userID === me.value?.id) return
      try { await api(\\\`/conversations/\\\${active.value}/poke\\\`, { method: 'POST', body: JSON.stringify({ user_id: userID }) }) } catch (cause) { showError(cause) }
    }
    // ---- direct info panel actions ----
    const directRemarkTarget = computed(() => (current.value?.kind === 'direct' ? friends.value.find(friend => friend.id === current.value.members.find(member => member.id !== me.value?.id)?.id) || null : null))
    async function startDirectRemark() {
      const target = directRemarkTarget.value
      if (!target) return
      const remark = prompt('设置好友备注（留空清除）', target.remark || '')
      if (remark === null) return
      try {
        await api(\\\`/friends/\\\${target.id}\\\`, { method: 'PATCH', body: JSON.stringify({ remark: remark.trim() }) })
        await refresh()
      } catch (cause) { showError(cause) }
    }
    async function clearLocalHistory() {
      const id = active.value
      if (!id || !confirm('确认清空本机的聊天记录？服务器与对方设备不受影响。')) return
      cache.delete(id)
      unreadCounts.value = { ...unreadCounts.value, [id]: 0 }
      visibleCount.value = 100
      showMessages(id)
    }
    // ---- conversation menu and private deletion ----`)

// 3. expose new bindings
patch(`    return { preferenceBusy, chatHeadMeta, memberMenuID, toggleMemberMenu,`,
`    return { memberByID, headMember, chatInfoOpen, dayDivider, dayLabel, pokeMember, startDirectRemark, clearLocalHistory, UserAvatar, chatHeadMeta, memberMenuID, toggleMemberMenu,`)

// 4. logout resets chatInfoOpen
patch(`showRequestComposer.value = false; showContacts.value = false;`,
`showRequestComposer.value = false; showContacts.value = false; chatInfoOpen.value = false;`)

fs.writeFileSync(path, src)
console.log('main.js round2 patched')
