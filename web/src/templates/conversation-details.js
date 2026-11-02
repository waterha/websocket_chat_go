// A runtime template fragment; whitespace and bindings belong to the root view.
export default `      <aside v-if="current && current.kind === 'group'" class="detail-panel group-detail-panel" aria-label="群信息"><LayoutDivider side="detail" />
        <section class="announcement-block" aria-label="群公告">
          <button type="button" class="announcement-block-head" @click="openAnnouncementPanel">
            <span class="announcement-block-title"><span class="detail-section-icon"><Bell :size="16" aria-hidden="true" /></span>群公告<span v-if="announcementCount" class="announcement-block-count">{{ announcementCount }}</span></span>
            <span class="announcement-block-more">全部<ChevronRight :size="13" aria-hidden="true" /></span>
          </button>
          <button type="button" class="announcement-summary" aria-label="查看群公告" @click="openAnnouncementPanel">
            <template v-if="latestAnnouncement"><span class="announcement-summary-label">最新公告</span><p class="announcement-block-preview">{{ latestAnnouncement.content }}</p><span class="announcement-summary-footer"><time :datetime="latestAnnouncement.created_at">{{ searchDate(latestAnnouncement.created_at) }}</time><span>查看详情 ↗</span></span></template>
            <template v-else><p class="announcement-block-empty">{{ canManageAnnouncements ? '让重要消息被每个人看见' : '群里还没有公告' }}</p><span class="announcement-summary-footer">{{ canManageAnnouncements ? '点击发布第一条公告' : '发布后会展示在这里' }}</span></template>
          </button>
        </section>
        <section class="member-block" aria-label="群成员">
          <header class="member-block-head"><span class="member-section-title"><span class="detail-section-icon"><UsersRound :size="16" aria-hidden="true" /></span>群成员 <b>{{ memberCount }}</b></span><button type="button" class="icon-button member-search-toggle" :aria-expanded="memberSearchOpen" title="搜索成员" aria-label="搜索成员" @click="toggleMemberSearch"><Search :size="15" aria-hidden="true" /></button></header>
          <div v-show="memberSearchOpen" class="member-block-search"><Search :size="15" aria-hidden="true" /><input v-model="memberQuery" type="search" maxlength="100" placeholder="搜索成员" aria-label="搜索群成员" @keydown.esc.prevent="memberQuery = ''; memberSearchOpen = false" /></div>
          <div class="member-block-list" role="list">
            <div v-for="person in currentMembers" :key="person.id" class="member-block-item" role="listitem">
              <span class="avatar-wrap tiny" :class="{ online: person.online }"><UserAvatar :user="person" :name="person.alias || person.name" size="tiny" /></span>
              <span class="member-block-text"><strong :title="person.alias || person.name">{{ person.alias || person.name }}<span v-if="person.id === me.id" class="member-self">我</span></strong><small :class="{ 'member-online-label': person.online }">{{ person.online ? '在线' : '离线' }}<span v-if="person.role !== 'member'" class="role-badge" :class="'role-' + person.role">{{ roleLabel(person.role) }}</span></small></span>
            </div>
            <p v-if="!currentMembers.length" class="empty-member">{{ memberQuery.trim() ? '没有匹配的成员' : '暂无群成员' }}</p>
          </div>
          <div class="member-block-actions">
            <button v-if="canInviteMembers" type="button" class="small-button" :disabled="memberCount >= 1000" @click="openInvite"><UserPlus :size="14" aria-hidden="true" />{{ memberCount >= 1000 ? '成员已满' : '邀请成员' }}</button>
            <button v-if="canManageGroupMembers" type="button" class="small-button" @click="openMembers"><Users :size="14" aria-hidden="true" />管理成员</button>
          </div>
        </section>
      </aside>
      <aside v-if="current && current.kind === 'direct' && chatInfoOpen" class="detail-panel direct-panel" aria-label="会话信息"><LayoutDivider side="detail" />
        <section class="direct-profile">
          <UserAvatar :user="headMember" :name="title(current)" size="profile-avatar" />
          <h3>{{ title(current) }}</h3>
          <p class="subtle">{{ headMember ? (headMember.online ? '在线' : '离线') : '' }}</p>
          <dl class="profile-details"><div><dt>账号</dt><dd>{{ headMember?.email }}</dd></div><div v-if="headMember && headMember.name !== title(current)"><dt>昵称</dt><dd>{{ headMember.name }}</dd></div></dl>
          <div class="profile-actions direct-actions">
            <button type="button" class="outline-button" @click="startDirectRemark">设置备注</button>
            <button v-if="headMember && !isBlocked(headMember.id)" type="button" class="outline-button" @click="blockUser(headMember)">加入黑名单</button>
            <button v-if="headMember && isBlocked(headMember.id)" type="button" class="outline-button" @click="unblockUser(headMember)">移出黑名单</button>
            <button type="button" class="outline-button" @click="deleteConversation(current.id)">删除会话</button>
            <button type="button" class="outline-button" @click="clearLocalHistory">清空聊天记录</button>
          </div>
          <p v-if="isBlocked(headMember?.id)" class="subtle">对方已在你的黑名单中，无法收到你的消息。</p>
        </section>
      </aside>
`
