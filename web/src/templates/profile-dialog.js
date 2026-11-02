// A runtime template fragment; whitespace and bindings belong to the root view.
export default `      <div v-if="showProfile" class="modal-backdrop" @click.self="showProfile = false">
        <div class="modal profile-modal" role="dialog" aria-modal="true" aria-label="个人信息">
          <div class="modal-head"><h2>个人信息</h2><button type="button" @click="showProfile = false" aria-label="关闭"><X :size="20" aria-hidden="true" /></button></div>
          <div class="profile-header">
            <UserAvatar :user="me" :name="profileEditing ? profileName : me.name" size="profile-avatar" />
            <div class="profile-avatar-controls"><h3>{{ me.name }}</h3><p class="profile-account">{{ me.email }}</p>
              <input ref="avatarInput" class="media-input" type="file" accept="image/png,image/jpeg,image/gif,image/webp,image/bmp,image/avif,image/heic,image/tiff" @change="uploadProfileAvatar" />
              <div class="profile-avatar-actions"><button class="small-button" type="button" :disabled="profileAvatarBusy" @click="avatarInput?.click()">{{ profileAvatarBusy ? '处理中…' : me.avatar ? '更换头像' : '上传头像' }}</button><button v-if="me.avatar" class="small-button reject-button" type="button" :disabled="profileAvatarBusy" @click="removeProfileAvatar">移除</button></div>
            </div>
          </div>
          <p class="profile-avatar-hint">支持拖动与缩放裁剪 · 原图最大 20 MB</p>
          <section class="profile-section" aria-label="基本资料">
            <header class="profile-section-head"><h3>基本资料</h3><button v-if="!profileEditing" class="profile-text-action" type="button" @click="profileEditing = true">编辑资料</button></header>
            <form v-if="profileEditing" class="profile-edit-form" @submit.prevent="updateProfile">
              <div class="profile-form"><label>用户名<input v-model="profileName" maxlength="64" autocomplete="name" :disabled="profileBusy" /></label><label>用户账号<input v-model="profileEmail" type="email" maxlength="254" autocomplete="email" :disabled="profileBusy" /></label></div>
              <div class="profile-actions"><button class="outline-button" type="button" :disabled="profileBusy" @click="profileEditing = false">取消</button><button class="primary" :disabled="profileBusy || !profileName.trim() || !profileEmail.trim()">{{ profileBusy ? '保存中…' : '保存修改' }}</button></div>
            </form>
            <dl v-else class="profile-details"><div><dt>用户名</dt><dd>{{ me.name }}</dd></div><div><dt>用户账号</dt><dd>{{ me.email }}</dd></div><div><dt>用户 ID</dt><dd class="profile-user-id">{{ me.id }}</dd></div></dl>
          </section>
          <section class="profile-section profile-security" aria-label="账号安全">
            <header class="profile-section-head"><h3>账号安全</h3></header>
            <div v-if="!passwordEditing" class="profile-security-row"><div><strong>登录密码</strong><p>定期更新密码，保护你的账号</p></div><button class="profile-text-action" type="button" @click="startPasswordChange">修改密码</button></div>
            <form v-else class="profile-edit-form" @submit.prevent="changePassword"><div class="profile-form"><label>当前密码<input v-model="currentPassword" type="password" maxlength="72" autocomplete="current-password" :disabled="passwordBusy" /></label><label>新密码<input v-model="newPassword" type="password" minlength="8" maxlength="72" autocomplete="new-password" placeholder="至少 8 位" :disabled="passwordBusy" /></label><label>确认新密码<input v-model="confirmPassword" type="password" minlength="8" maxlength="72" autocomplete="new-password" :disabled="passwordBusy" /></label></div><p class="profile-security-hint">修改后，其他设备需重新登录，当前设备保持登录。</p><div class="profile-actions"><button class="outline-button" type="button" :disabled="passwordBusy" @click="passwordEditing = false">取消</button><button class="primary" :disabled="passwordBusy || !currentPassword || !newPassword || !confirmPassword">{{ passwordBusy ? '修改中…' : '更新密码' }}</button></div></form>
          </section>
        </div>
      </div>
`
