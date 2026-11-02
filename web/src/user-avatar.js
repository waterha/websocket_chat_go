import { computed } from 'vue'
import { mediaURL } from './media.js'

export const UserAvatar = {
  props: { user: { type: Object, default: null }, name: { type: String, default: '?' }, size: { type: String, default: '' } },
  setup(props) {
    const url = computed(() => (props.user?.avatar ? mediaURL(props.user.avatar) : ''))
    return { url }
  },
  template: '<img :src="url || \'/default-avatar.svg\'" class="avatar" :class="size" :alt="url ? \'\' : \'默认头像\'" loading="lazy" />',
}
