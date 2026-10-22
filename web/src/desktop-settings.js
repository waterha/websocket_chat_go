import { ref } from 'vue/dist/vue.esm-bundler.js'
import { checkServer, desktopConfig, isDesktop, saveServer } from './desktop.js'

export const DesktopSettings = {
  props: { loggedIn: Boolean },
  setup() {
    const enabled = isDesktop()
    const current = desktopConfig() || { serverURL: '', origin: '' }
    const opened = ref(false)
    const serverURL = ref(current.serverURL)
    const origin = ref(current.origin)
    const busy = ref(false)
    const status = ref('')
    const succeeded = ref(false)
    const open = () => { serverURL.value = current.serverURL; origin.value = current.origin; status.value = ''; opened.value = true }
    const serverChanged = () => { origin.value = ''; status.value = '' }
    async function act(save) {
      if (busy.value) return
      busy.value = true
      succeeded.value = false
      status.value = ''
      try {
        const value = { serverURL: serverURL.value.trim(), origin: origin.value.trim() }
        if (save) {
          await saveServer(value)
          location.reload()
        } else {
          await checkServer(value)
          succeeded.value = true
          status.value = '连接正常，服务器已就绪。'
        }
      } catch (error) { status.value = String(error?.message || error) }
      finally { busy.value = false }
    }
    return { enabled, current, opened, serverURL, origin, busy, status, succeeded, open, serverChanged, act }
  },
  template: `
    <template v-if="enabled">
      <header class="desktop-bar"><span><span class="desktop-indicator"></span>协作聊天 · 桌面端</span><button type="button" @click="open" aria-haspopup="dialog">{{ current.serverURL }} · 服务器设置</button></header>
      <div v-if="opened" class="modal-backdrop desktop-settings-backdrop" @click.self="!busy && (opened = false)" @keydown.esc="!busy && (opened = false)">
        <section class="modal desktop-settings" role="dialog" aria-modal="true" aria-labelledby="desktop-settings-title">
          <div class="modal-head"><h2 id="desktop-settings-title">连接服务器</h2><button type="button" :disabled="busy" @click="opened = false" aria-label="关闭">×</button></div>
          <p class="subtle">连接团队服务器后，即可登录并同步聊天记录。</p>
          <p v-if="loggedIn" class="subtle">退出登录后可切换服务器。</p>
          <form @submit.prevent="act(true)">
            <label>服务器地址<input v-model="serverURL" @input="serverChanged" type="url" required placeholder="https://chat.example.com" :disabled="busy || loggedIn" /></label>
            <details><summary>高级设置</summary><label>服务器允许的网站地址<input v-model="origin" type="url" placeholder="留空则使用服务器地址" :disabled="busy || loggedIn" /></label><p class="subtle">与服务端 FRONTEND_ORIGIN 一致。本机默认 http://127.0.0.1:4173。</p></details>
            <p v-if="status" class="desktop-settings-status" :class="{ success: succeeded }" role="status">{{ status }}</p>
            <div class="desktop-settings-actions"><button type="button" class="outline-button" :disabled="busy" @click="act(false)">{{ busy ? '处理中…' : '测试连接' }}</button><button class="primary" :disabled="busy || loggedIn">保存并连接</button></div>
          </form>
        </section>
      </div>
    </template>`,
}
