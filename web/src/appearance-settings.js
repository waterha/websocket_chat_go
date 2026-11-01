import { ref, onMounted, onUnmounted } from 'vue/dist/vue.esm-bundler.js'
import { X, Check, Palette } from 'lucide-vue-next'
import { applyTheme, defaultColor, loadTheme, saveTheme, themePresets } from './appearance-theme.js'

export const AppearanceSettings = {
  components: { X, Check, Palette },
  emits: ['close'],
  setup() {
    const color = ref(loadTheme())
    const status = ref('')
    const panel = ref(null)
    const previousFocus = document.activeElement
    function select(value) {
      color.value = applyTheme(value)
      try {
        saveTheme(color.value)
        status.value = '配色已保存，下次打开仍会使用。'
      } catch {
        status.value = '配色已应用，但本机存储不可用，重新打开后可能恢复默认。'
      }
    }
    function trapFocus(event) {
      if (event.key !== 'Tab') return
      const items = [...panel.value.querySelectorAll('button, input')].filter(item => !item.disabled)
      const first = items[0], last = items.at(-1)
      if (event.shiftKey && document.activeElement === first) { event.preventDefault(); last.focus() }
      else if (!event.shiftKey && document.activeElement === last) { event.preventDefault(); first.focus() }
    }
    onMounted(() => panel.value.querySelector('button').focus())
    onUnmounted(() => { if (previousFocus?.isConnected) previousFocus.focus() })
    return { color, status, panel, select, trapFocus, themePresets, defaultColor }
  },
  template: `
    <div class="modal-backdrop appearance-backdrop" @click.self="$emit('close')" @keydown.esc.stop.prevent="$emit('close')" @keydown="trapFocus">
      <section ref="panel" class="modal appearance-modal" role="dialog" aria-modal="true" aria-labelledby="appearance-title" aria-describedby="appearance-description">
        <header class="modal-head"><h2 id="appearance-title"><Palette :size="21" aria-hidden="true" />美化界面</h2><button type="button" aria-label="关闭美化界面" @click="$emit('close')"><X :size="21" aria-hidden="true" /></button></header>
        <p id="appearance-description" class="subtle">挑选喜欢的色调，侧边栏、聊天气泡和按钮会一起变换。即时生效，自动保存在本机。</p>
        <div class="appearance-presets" role="group" aria-label="预设色调">
          <button v-for="preset in themePresets" :key="preset.color" type="button" class="appearance-preset" :aria-pressed="color === preset.color" @click="select(preset.color)">
            <span class="appearance-swatch" :style="{ background: preset.color }"><Check v-if="color === preset.color" :size="20" aria-hidden="true" /></span><span>{{ preset.name }}</span>
          </button>
        </div>
        <label class="appearance-custom"><span>自定义颜色<small>选择任意颜色，自动搭配柔和背景与清晰的文字。</small></span><input type="color" aria-label="自定义界面色调" :value="color" @input="select($event.target.value)" /></label>
        <div class="appearance-preview" aria-label="当前配色预览"><span class="appearance-preview-caption">配色预览</span><div class="appearance-preview-bubble">让聊天界面多一点你的风格。</div><div class="appearance-preview-bubble mine">就用这个颜色！</div></div>
        <p class="appearance-status" role="status" aria-live="polite">{{ status || '可随时切换，聊天内容不受影响。' }}</p>
        <footer class="appearance-actions"><button type="button" class="outline-button" @click="select(defaultColor)">恢复默认</button><button type="button" class="primary" @click="$emit('close')">完成</button></footer>
      </section>
    </div>`,
}
