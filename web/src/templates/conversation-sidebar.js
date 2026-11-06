// A runtime template fragment; whitespace and bindings belong to the root view.
export default `      <aside class="sidebar"><LayoutDivider side="sidebar" />
          <header class="side-top">
            <span class="side-top-title"><MessageCircle :size="17" aria-hidden="true" />消息</span>
            <div class="sidebar-more">
              <button type="button" class="head-icon" :class="{ dot: pendingIncomingRequests.length }" title="更多" aria-label="更多操作" :aria-expanded="sidebarMoreOpen" @click.stop="toggleSidebarMore"><MoreHorizontal :size="18" /></button>
              <div v-if="sidebarMoreOpen" class="top-menu" role="menu" @click.stop>
                <button type="button" role="menuitem" @click="openGlobalSearch(); sidebarMoreOpen = false"><Search :size="15" />全局搜索</button>
                <button type="button" role="menuitem" @click="openAddUser(); sidebarMoreOpen = false"><UserPlus :size="15" />添加好友</button>
                <button type="button" role="menuitem" @click="openFriendRequests(); sidebarMoreOpen = false"><Bell :size="15" />好友申请<span v-if="pendingIncomingRequests.length" class="request-count">{{ pendingIncomingRequests.length > 99 ? '99+' : pendingIncomingRequests.length }}</span></button>
                <button type="button" role="menuitem" @click="openContacts(); sidebarMoreOpen = false"><Contact :size="15" />联系人管理</button>
                <button type="button" role="menuitem" @click="openCreate(); sidebarMoreOpen = false"><UsersRound :size="15" />创建群聊</button>
                <button type="button" role="menuitem" @click="openGroupDirectory()"><Users :size="15" />加入群聊</button>
              </div>
            </div>
          </header>
          <div class="conversation-tabs"><button :class="{ active: conversationFilter === 'current' }" @click="conversationFilter = 'current'">当前</button><button :class="{ active: conversationFilter === 'friends' }" @click="conversationFilter = 'friends'">好友</button><button :class="{ active: conversationFilter === 'group' }" @click="conversationFilter = 'group'">群聊</button></div>
          <div class="directory-search" role="search" aria-label="查找好友和群聊"><Search :size="17" aria-hidden="true" /><input ref="directoryInput" v-model="directoryQuery" type="search" maxlength="100" :placeholder="directoryPlaceholder" :aria-label="directoryPlaceholder" @keydown.esc.prevent="clearDirectorySearch" /><button v-if="directoryQuery" type="button" class="icon-button" title="清除搜索" aria-label="清除列表搜索" @click="clearDirectorySearch"><X :size="16" aria-hidden="true" /></button></div>
          <GroupDirectory ref="groupDirectory" :show-trigger="false" @joined="onGroupJoined" />
          <div class="conversation-list">
             <p v-if="directoryQuery.trim() && !directoryCount" class="empty-side" role="status">{{ conversationFilter === 'friends' ? '没有匹配的好友' : conversationFilter === 'group' ? '没有匹配的群聊' : '没有匹配的当前会话' }}</p>
            <template v-if="conversationFilter === 'friends'">
              <button v-for="person in filteredFriends" :key="person.id" class="conversation contact" @click="openDirect(person.id)">
                <UserAvatar :user="person" :name="person.remark || person.name" /><span class="conversation-text"><strong>{{ person.remark || person.name }}</strong><small>{{ person.remark ? person.name : contactPreview(person) }}</small></span><span v-if="directConversation(person.id) && showsBadge(directConversation(person.id))" class="unread-badge">{{ unreadLabel(directConversation(person.id).id) }}</span>
              </button>
            </template>
            <template v-else-if="conversationFilter === 'group'">
              <button v-for="item in filteredGroups" :key="item.id" class="conversation" :class="{ active: active === item.id }" @click="select(item.id)" @contextmenu.prevent.stop="openConversationMenu($event, item)">
                <UserAvatar v-if="item.kind === 'direct'" :user="conversationAvatar(item)" :name="title(item)" /><span v-else class="avatar">{{ title(item).slice(0, 1) }}</span><span class="conversation-text"><strong>{{ title(item) }}<span v-if="item.dismissed" class="mute-mark" title="已解散" aria-label="已解散">已解散</span><span v-if="item.pinned" class="pin-mark" title="已置顶" aria-label="已置顶">置顶</span><span v-if="item.muted" class="mute-mark" title="免打扰" aria-label="免打扰">免打扰</span></strong><small>{{ preview(item) }}</small></span><span v-if="item.pending_joins" class="unread-badge pending-joins" title="待审批的入群邀请">{{ item.pending_joins > 99 ? '99+' : item.pending_joins }}</span><span v-if="showsBadge(item)" class="unread-badge">{{ unreadLabel(item.id) }}</span>
              </button>
            </template>
            <template v-else>
              <p v-if="pinnedConversations.length && !directoryQuery.trim()" class="list-section-title">置顶会话</p>
              <button v-for="item in pinnedConversations" :key="'pin-' + item.id" class="conversation" :class="{ active: active === item.id }" @click="select(item.id)" @contextmenu.prevent.stop="openConversationMenu($event, item)">
                <UserAvatar v-if="item.kind === 'direct'" :user="conversationAvatar(item)" :name="title(item)" /><span v-else class="avatar">{{ title(item).slice(0, 1) }}</span><span class="conversation-text"><strong>{{ title(item) }}<span class="pin-mark" title="已置顶" aria-label="已置顶">置顶</span><span v-if="item.muted" class="mute-mark" title="免打扰" aria-label="免打扰">免打扰</span></strong><small>{{ preview(item) }}</small></span><span v-if="showsBadge(item)" class="unread-badge">{{ unreadLabel(item.id) }}</span>
              </button>
              <p v-if="pinnedConversations.length && filteredRecent.length && !directoryQuery.trim()" class="list-section-title">最近</p>
              <button v-for="item in filteredRecent" :key="item.id" class="conversation" :class="{ active: active === item.id }" @click="select(item.id)" @contextmenu.prevent.stop="openConversationMenu($event, item)">
                <UserAvatar v-if="item.kind === 'direct'" :user="conversationAvatar(item)" :name="title(item)" /><span v-else class="avatar">{{ title(item).slice(0, 1) }}</span><span class="conversation-text"><strong>{{ title(item) }}<span v-if="item.muted" class="mute-mark" title="免打扰" aria-label="免打扰">免打扰</span></strong><small>{{ preview(item) }}</small></span><span v-if="showsBadge(item)" class="unread-badge">{{ unreadLabel(item.id) }}</span>
              </button>
            </template>
            <p v-if="conversationFilter === 'friends' && !friends.length && !directoryQuery.trim()" class="empty-side">还没有好友，点击“添加”开始。</p>
            <p v-else-if="conversationFilter === 'group' && !groupConversations.length && !directoryQuery.trim()" class="empty-side">还没有加入群聊。</p>
            <p v-else-if="conversationFilter === 'current' && !recentConversations.length && !directoryQuery.trim()" class="empty-side">还没有最近会话。</p>
         </div>
        <footer class="sidebar-footer"><UserAvatar :user="me" :name="me.name" size="self" /><span class="identity"><strong>{{ me.name }}</strong><small>{{ connected ? '实时连接已建立' : '正在重连 · 历史可用' }}</small></span><button @click="showSettings = !showSettings" title="设置" aria-haspopup="menu" :aria-expanded="showSettings">设置</button><div v-if="showSettings" class="settings-menu" role="menu"><button type="button" role="menuitem" @click="openProfile">个人</button><button type="button" role="menuitem" @click="showSettings = false; showAppearance = true">美化界面</button><button type="button" role="menuitem" @click="showSettings = false; showFavorites = true">收藏</button><button type="button" role="menuitem" :disabled="notificationPermission === 'granted'" @click="enableSystemNotifications">{{ notificationPermission === 'granted' ? '系统通知已开启' : '开启系统通知' }}</button><button type="button" role="menuitem" @click="logout">退出</button></div></footer>
      </aside>
`
