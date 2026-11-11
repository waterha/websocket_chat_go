// A runtime template fragment; whitespace and bindings belong to the root view.
export default `      <div v-if="showInvite" class="modal-backdrop invite-backdrop" @click.self="showInvite = false">
        <div class="invite-modal" role="dialog" aria-modal="true" aria-labelledby="invite-title">
          <header class="invite-head">
            <div class="invite-head-text">
              <h2 id="invite-title">邀请成员</h2>
              <p>{{ current?.name }} · 当前 {{ memberCount }} 人，群上限 1000 人</p>
            </div>
            <button type="button" class="invite-close" @click="showInvite = false" aria-label="关闭">×</button>
          </header>

          <div class="invite-controls">
            <label class="invite-search">
              <Search :size="16" aria-hidden="true" />
              <input v-model="inviteQuery" type="search" maxlength="100" placeholder="搜索用户名或账号" aria-label="搜索要邀请的用户" />
              <button v-if="inviteQuery" type="button" class="invite-search-clear" title="清除搜索" aria-label="清除搜索" @click.prevent="inviteQuery = ''"><X :size="14" aria-hidden="true" /></button>
            </label>
            <div class="invite-index" role="group" aria-label="按首字母筛选">
              <button type="button" class="invite-letter" :class="{ active: !inviteInitial }" :aria-pressed="!inviteInitial" @click="inviteInitial = ''">全部</button>
              <button v-for="initial in inviteInitials" :key="initial" type="button" class="invite-letter" :class="{ active: inviteInitial === initial }" :aria-pressed="inviteInitial === initial" @click="chooseInviteInitial(initial)">{{ initial === OTHER_INITIAL ? '其他' : initial }}</button>
            </div>
          </div>

          <div v-if="inviteSelectedCount" class="invite-chips">
            <span class="invite-chips-label">已选 {{ inviteSelectedCount }} 人</span>
            <div class="invite-chips-list">
              <button v-for="person in inviteSelectedPeople" :key="person.id" type="button" class="invite-chip" :title="'移出 ' + person.name" @click="toggleInvite(person)">{{ person.name }}<X :size="12" aria-hidden="true" /></button>
            </div>
            <button type="button" class="invite-chips-clear" @click="clearInvites">清空</button>
          </div>

          <div class="invite-list" role="listbox" aria-multiselectable="true" aria-label="可邀请的用户">
            <div v-for="group in inviteGroups" :key="group.initial" class="invite-group">
              <div class="invite-group-head">{{ group.initial === OTHER_INITIAL ? '其他' : group.initial }}</div>
              <button v-for="person in group.people" :key="person.id" type="button" class="invite-row" role="option" :aria-selected="inviteUsers.includes(person.id)" :class="{ selected: inviteUsers.includes(person.id) }" @click="toggleInvite(person)">
                <span class="avatar tiny">{{ person.name.slice(0,1) }}</span>
                <span class="invite-row-text"><strong>{{ person.name }}</strong><small>{{ person.email }}</small></span>
                <span class="invite-check" aria-hidden="true"><Check :size="14" /></span>
              </button>
            </div>
            <ContentState v-if="!inviteVisible.length" class="invite-empty" compact :description="inviteCandidates.length ? '没有匹配的用户，试试其它关键词或首字母。' : '暂无可邀请的用户。'" />
          </div>

          <footer class="invite-foot">
            <div class="invite-foot-info">
              <span class="invite-foot-count">{{ inviteSelectedCount }}<span> / {{ inviteSummary.remaining }} 可选</span></span>
              <button type="button" class="invite-link" :disabled="!inviteVisible.length" @click="selectAllInvites">全选当前结果</button>
            </div>
            <div class="invite-foot-buttons">
              <button type="button" class="outline-button" @click="showInvite = false">取消</button>
              <button class="primary" :disabled="conversationBusy || inviteSummary.disabled" @click="addMembers">{{ conversationBusy ? '邀请中…' : inviteSummary.label }}</button>
            </div>
          </footer>
        </div>
      </div>
`
