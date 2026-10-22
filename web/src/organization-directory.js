import { ref, nextTick, onUnmounted } from 'vue/dist/vue.esm-bundler.js'
import { api } from './api.js'

export const OrganizationDirectory = {
  emits: ['chat'],
  setup(_, { emit }) {
    const opened = ref(false), panel = ref(null), trigger = ref(null)
    const query = ref(''), department = ref(''), entries = ref([]), departments = ref([])
    const busy = ref(false), error = ref(''), hasMore = ref(false), nextOffset = ref(0)
    const profileReady = ref(false), profileBusy = ref(false), profileError = ref(''), saved = ref(false)
    const workDepartment = ref(''), jobTitle = ref('')
    const appliedQuery = ref(''), appliedDepartment = ref('')
    let directoryRequest, profileRequest, generation = 0, profileGeneration = 0, lifetime = 0

    function cancel() {
      generation++; profileGeneration++; lifetime++
      directoryRequest?.abort(); profileRequest?.abort()
    }
    async function load(append = false) {
      directoryRequest?.abort()
      const version = ++generation
      directoryRequest = new AbortController()
      if (!append) {
        entries.value = []; hasMore.value = false; nextOffset.value = 0
        appliedQuery.value = query.value.trim(); appliedDepartment.value = department.value
      }
      busy.value = true; error.value = ''
      try {
        const params = new URLSearchParams({ q: appliedQuery.value, department: appliedDepartment.value, offset: String(nextOffset.value), limit: '50' })
        const page = await api(`/directory?${params}`, { signal: directoryRequest.signal })
        if (version !== generation) return
        const known = new Set(append ? entries.value.map(entry => entry.id) : [])
        entries.value = [...(append ? entries.value : []), ...page.entries.filter(entry => !known.has(entry.id))]
        departments.value = page.departments
        hasMore.value = page.has_more; nextOffset.value = page.next_offset
      } catch (cause) {
        if (version === generation && cause.name !== 'AbortError') error.value = '通讯录加载失败，请确认服务器已更新后重试。'
      } finally { if (version === generation) busy.value = false }
    }
    async function loadProfile() {
      profileRequest?.abort(); profileRequest = new AbortController()
      const version = ++profileGeneration
      profileReady.value = false; profileBusy.value = true; profileError.value = ''
      try {
        const profile = await api('/directory/me', { signal: profileRequest.signal })
        if (version !== profileGeneration) return
        workDepartment.value = profile.department; jobTitle.value = profile.job_title; profileReady.value = true
      } catch (cause) {
        if (version === profileGeneration && cause.name !== 'AbortError') profileError.value = '工作资料加载失败，请重试。'
      } finally { if (version === profileGeneration) profileBusy.value = false }
    }
    async function saveProfile() {
      if (profileBusy.value || !profileReady.value) return
      const version = lifetime
      profileBusy.value = true; profileError.value = ''; saved.value = false
      try {
        const profile = await api('/directory/me', { method: 'PATCH', body: JSON.stringify({ department: workDepartment.value, job_title: jobTitle.value }) })
        if (version !== lifetime) return
        workDepartment.value = profile.department; jobTitle.value = profile.job_title; saved.value = true
      } catch (cause) {
        if (version === lifetime) profileError.value = cause.code === 'invalid_work_profile' ? '部门和职位各限 100 字，请勿输入控制字符。' : '保存失败，请重试。'
      } finally { if (version === lifetime) profileBusy.value = false }
    }
    async function open() {
      opened.value = true; query.value = ''; department.value = ''; departments.value = []
      saved.value = false; workDepartment.value = ''; jobTitle.value = ''
      load(); loadProfile()
      await nextTick(); panel.value?.querySelector('input[type="search"]')?.focus()
    }
    function close() {
      if (profileBusy.value && profileReady.value) return
      cancel(); opened.value = false; entries.value = []; departments.value = []
      nextTick(() => trigger.value?.focus())
    }
    function chat(id) { close(); emit('chat', id) }
    function keyboard(event) {
      if (event.key === 'Escape') { event.preventDefault(); event.stopPropagation(); close() }
      if (event.key !== 'Tab') return
      const focusable = [...panel.value.querySelectorAll('button,input,select,a[href],summary')].filter(el => !el.disabled && !el.closest('details:not([open]) > :not(summary)'))
      const first = focusable[0], last = focusable.at(-1)
      if (event.shiftKey && document.activeElement === first) { event.preventDefault(); last?.focus() }
      if (!event.shiftKey && document.activeElement === last) { event.preventDefault(); first?.focus() }
    }
    onUnmounted(cancel)
    return { opened, panel, trigger, query, department, entries, departments, busy, error, hasMore, nextOffset,
      profileReady, profileBusy, profileError, saved, workDepartment, jobTitle, appliedQuery, appliedDepartment,
      open, close, load, loadProfile, saveProfile, chat, keyboard }
  },
  template: `
    <button ref="trigger" type="button" class="org-entry" aria-haspopup="dialog" @click="open">同事通讯录 <span>查找同事 · 发起聊天</span></button>
    <Teleport to="body"><div v-if="opened" class="modal-backdrop org-backdrop" @click.self="close" @keydown="keyboard">
      <section ref="panel" class="modal org-panel" role="dialog" aria-modal="true" aria-labelledby="org-title">
        <div class="modal-head"><div><h2 id="org-title">同事通讯录</h2><p class="subtle">查找本团队服务器上的同事，直接开始沟通。</p></div><button type="button" aria-label="关闭通讯录" :disabled="profileBusy && profileReady" @click="close">×</button></div>
        <details class="org-profile"><summary>我的工作资料</summary>
          <p class="subtle">部门与职位由本人填写，供同事查找。</p>
          <form @submit.prevent="saveProfile">
            <label>部门<input v-model="workDepartment" maxlength="100" :disabled="!profileReady || profileBusy" @input="saved = false" placeholder="例如：产品研发部" /></label>
            <label>职位<input v-model="jobTitle" maxlength="100" :disabled="!profileReady || profileBusy" @input="saved = false" placeholder="例如：产品经理" /></label>
            <button class="primary" :disabled="!profileReady || profileBusy">{{ profileBusy ? '处理中…' : '保存工作资料' }}</button>
          </form>
          <p v-if="profileError" role="alert" class="org-error">{{ profileError }} <button v-if="!profileReady" type="button" :disabled="profileBusy" @click="loadProfile">重试加载资料</button></p>
          <p v-if="saved" role="status">工作资料已保存</p>
        </details>
        <form class="org-search" role="search" @submit.prevent="load()">
          <label>查找同事<input v-model="query" type="search" maxlength="200" placeholder="姓名、账号、部门或职位" /></label>
          <label>部门<select v-model="department" @change="load()"><option value="">全部部门</option><option v-for="name in departments" :key="name" :value="name">{{ name }}</option></select></label>
          <button class="primary" type="submit">搜索</button>
        </form>
        <p v-if="error" role="alert" class="org-error">{{ error }} <button type="button" :disabled="busy" @click="load(entries.length > 0)">重试</button></p>
        <div class="org-results" :aria-busy="busy">
          <p v-if="busy" role="status">正在加载同事…</p>
          <p v-else-if="!entries.length && !error" role="status">没有找到同事。可更换关键词或选择全部部门。</p>
          <ul class="org-list"><li v-for="person in entries" :key="person.id" class="org-person">
            <span class="avatar" aria-hidden="true">{{ person.name.slice(0,1) }}</span>
            <div class="org-info"><strong>{{ person.name }}</strong><span>{{ person.department || '未填写部门' }} · {{ person.job_title || '未填写职位' }}</span><small>{{ person.email }}</small></div>
            <button type="button" class="outline-button" :disabled="profileBusy && profileReady" :aria-label="'与' + person.name + '聊天'" @click="chat(person.id)">发消息</button>
          </li></ul>
          <button v-if="hasMore" type="button" class="outline-button org-more" :disabled="busy || query.trim() !== appliedQuery || department !== appliedDepartment" @click="load(true)">加载更多同事</button>
        </div>
      </section>
    </div></Teleport>`,
}
