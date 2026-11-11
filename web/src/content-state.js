import { computed } from 'vue/dist/vue.esm-bundler.js'
import { Inbox, LoaderCircle, CircleAlert, RotateCw } from 'lucide-vue-next'
import './content-state.css'

// Shared feedback surface: retain context in description, keep headings/actions
// consistent, and never announce decorative SVGs as duplicate status text.
export default {
  name: 'ContentState',
  components: { Inbox, LoaderCircle, CircleAlert, RotateCw },
  props: {
    kind: { type: String, default: 'empty', validator: value => ['empty', 'loading', 'error'].includes(value) },
    title: { type: String, default: '' },
    description: { type: String, default: '' },
    compact: Boolean,
    retryLabel: { type: String, default: '重试' },
    retryDisabled: Boolean,
  },
  emits: ['retry'],
  setup(props) {
    const heading = computed(() => props.title || ({ empty: '暂无内容', loading: '加载中…', error: '加载失败' }[props.kind]))
    return { heading }
  },
  template: `<div class="content-state" :class="['content-state--' + kind, { 'content-state--compact': compact }]" :role="kind === 'error' ? 'alert' : 'status'" :aria-busy="kind === 'loading' ? 'true' : undefined" aria-atomic="true"><span class="content-state__icon" aria-hidden="true"><LoaderCircle v-if="kind === 'loading'" :size="24" class="content-state__spinner" /><CircleAlert v-else-if="kind === 'error'" :size="24" /><Inbox v-else :size="24" /></span><div class="content-state__copy"><strong class="content-state__title">{{ heading }}</strong><p v-if="description" class="content-state__description">{{ description }}</p></div><button v-if="kind === 'error'" type="button" class="outline-button content-state__retry" :disabled="retryDisabled" @click="$emit('retry')"><RotateCw :size="15" aria-hidden="true" />{{ retryLabel }}</button></div>`,
}
