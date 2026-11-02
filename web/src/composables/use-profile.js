import { ref, watch, onUnmounted } from 'vue/dist/vue.esm-bundler.js'
import { api, clearAvatar as clearProfileAvatar, MAX_AVATAR_BYTES, uploadAvatar } from '../api.js'
import { prepareImage } from '../image-input.js'
import { cropAvatar } from '../avatar-crop.js'

export function useProfile({ me, showSettings, refresh, showError, convertImageToPNG, getEpoch }) {
  const showProfile = ref(false)
  const profileEditing = ref(false)
  const profileBusy = ref(false)
  const profileName = ref('')
  const profileEmail = ref('')
  const profileAvatarBusy = ref(false)
  const avatarInput = ref(null)
  const passwordEditing = ref(false)
  const passwordBusy = ref(false)
  const currentPassword = ref('')
  const newPassword = ref('')
  const confirmPassword = ref('')
  function openProfile() {
    showSettings.value = false
    profileName.value = me.value?.name || ''
    profileEmail.value = me.value?.email || ''
    profileEditing.value = false
    showProfile.value = true
  }
  async function updateProfile() {
    if (profileBusy.value) return
    profileBusy.value = true
    try {
      const account = await api('/me', { method: 'PATCH', body: JSON.stringify({ email: profileEmail.value, name: profileName.value }) })
      me.value = account
      profileEditing.value = false
      await refresh()
    } catch (cause) { showError(cause) }
    finally { profileBusy.value = false }
  }
  let avatarCropController
  watch(showProfile, visible => { if (!visible) avatarCropController?.abort() })
  async function uploadProfileAvatar(event) {
    const input = event?.target
    const file = input?.files?.[0]
    if (input) input.value = ''
    if (!file || profileAvatarBusy.value) return
    profileAvatarBusy.value = true
    const controller = new AbortController()
    avatarCropController = controller
    const version = getEpoch()
    try {
      if (file.size > 20 * 1024 * 1024) throw new Error('待裁剪图片不能超过 20 MB')
      const prepared = await prepareImage(file, convertImageToPNG)
      if (!['image/jpeg', 'image/png', 'image/gif', 'image/webp'].includes(prepared.type)) {
        throw new Error('头像仅支持 PNG、JPEG、GIF 或 WebP 图片')
      }
      if (!prepared.size) throw new Error('头像图片不能为空')
      const cropped = await cropAvatar(prepared, { signal: controller.signal })
      if (!cropped || controller.signal.aborted || version !== getEpoch()) return
      if (cropped.size > MAX_AVATAR_BYTES) throw new Error('裁剪后的头像不能超过 2 MB')
      const account = await uploadAvatar(cropped)
      if (controller.signal.aborted || version !== getEpoch()) return
      me.value = account
      await refresh()
    } catch (cause) { if (!controller.signal.aborted && version === getEpoch()) showError(cause) }
    finally { if (avatarCropController === controller) { avatarCropController = null; profileAvatarBusy.value = false } }
  }
  async function removeProfileAvatar() {
    if (profileAvatarBusy.value || !me.value?.avatar) return
    if (!confirm('确认移除当前头像？')) return
    profileAvatarBusy.value = true
    try {
      me.value = await clearProfileAvatar()
      await refresh()
    } catch (cause) { showError(cause) }
    finally { profileAvatarBusy.value = false }
  }
  function startPasswordChange() {
    currentPassword.value = ''
    newPassword.value = ''
    confirmPassword.value = ''
    passwordEditing.value = true
  }
  // Changing the password revokes every other session, so the user is told what
  // will happen before the request is sent.
  async function changePassword() {
    if (passwordBusy.value) return
    if (newPassword.value !== confirmPassword.value) { showError({ message: '两次输入的新密码不一致' }); return }
    if ([...newPassword.value].length < 8) { showError({ message: '新密码至少 8 位' }); return }
    passwordBusy.value = true
    try {
      await api('/auth/password', { method: 'POST', body: JSON.stringify({ current_password: currentPassword.value, new_password: newPassword.value }) })
      passwordEditing.value = false
      currentPassword.value = ''
      newPassword.value = ''
      confirmPassword.value = ''
      alert('密码已修改。其他设备上的登录已被退出，当前会话保持登录。')
    } catch (cause) { showError(cause) }
    finally { passwordBusy.value = false }
  }
  onUnmounted(() => avatarCropController?.abort())

  return {
    showProfile, profileEditing, profileBusy, profileName, profileEmail, profileAvatarBusy, avatarInput,
    passwordEditing, passwordBusy, currentPassword, newPassword, confirmPassword, openProfile,
    updateProfile, uploadProfileAvatar, removeProfileAvatar, startPasswordChange, changePassword,
  }
}
