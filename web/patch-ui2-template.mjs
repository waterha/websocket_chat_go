// One-off patch: template round 2 (avatars, dividers, direct panel, poke).
import fs from 'node:fs'

const path = 'src/template.js'
let src = fs.readFileSync(path, 'utf8').split('\r\n').join('\n')

function patch(old, next) {
  if (!src.includes(old)) throw new Error('MISSING: ' + old.slice(0, 90))
  src = src.replace(old, next)
}

// 1. sidebar footer self avatar
patch(`<footer class="sidebar-footer"><span class="avatar self">{{ me.name.slice(0,1) }}</span>`,
  `<footer class="sidebar-footer"><UserAvatar :user="me" :name="me.name" size="self" />`)

// 2. chat-head avatar: direct uses the other member's avatar
patch(`           <header class="chat-head" aria-label="会话信息">
             <span class="avatar">{{ title(current).slice(0, 1) }}</span>`,
  `           <header class="chat-head" aria-label="会话信息">
             <UserAvatar v-if="current.kind === 'direct'" :user="headMember" :name="title(current)" />
             <span v-else class="avatar">{{ title(current).slice(0, 1) }}</span>`)
patch(`<button type="button" @click="openSearch">搜索</button>
               <button v-if="current.kind === 'group'" type="button" @click="openMembers">成员</button>`,
  `<button type="button" @click="openSearch">搜索</button>
               <button v-if="current.kind === 'group'" type="button" @click="openMembers">成员</button>
               <button v-if="current.kind === 'direct'" type="button" :aria-expanded="chatInfoOpen" @click="chatInfoOpen = !chatInfoOpen">信息</button>`)

// 3. message rows: sender avatar for others, own avatar on the right, date divider
patch(`<div v-for="item in messages" :key="item.id" class="message-row"`,
  `<template v-for="(item, index) in messages" :key="item.id">
              <p v-if="dayDivider(index)" class="day-divider" role="separator">{{ dayLabel(item.created_at) }}</p>
              <div class="message-row"`)
patch(`@click.capture="messageActions.onMessageClick($event, item)">
              <button v-if="messageActions.selectionMode && item.kind !== 'system' && !item.recalled_at"`,
  `@click.capture="messageActions.onMessageClick($event, item)">
              <UserAvatar v-if="item.sender_id !== me.id && item.kind !== 'system'" :user="memberByID(item.sender_id)" :name="sender(item.sender_id)" size="tiny" />
              <button v-if="messageActions.selectionMode && item.kind !== 'system' && !item.recalled_at"`)
patch(`<template v-else>{{ bodyOf(item) }}</template></template></div><div v-if="item.reactions?.length && !item.recalled_at" class="reaction-row"><button v-for="entry in item.reactions" :key="entry.emoji" type="button" class="reaction-chip" :class="{ mine: entry.users.includes(me.id) }" :disabled="reactionBusy" :aria-label="(entry.users.includes(me.id) ? '取消' : '添加') + entry.emoji + '回应'" @click.stop="toggleReaction(item, entry.emoji)">{{ entry.emoji }} {{ entry.count }}</button></div></div></template>
            </div>
          </section>`,
  `<template v-else>{{ bodyOf(item) }}</template></template></div><div v-if="item.reactions?.length && !item.recalled_at" class="reaction-row"><button v-for="entry in item.reactions" :key="entry.emoji" type="button" class="reaction-chip" :class="{ mine: entry.users.includes(me.id) }" :disabled="reactionBusy" :aria-label="(entry.users.includes(me.id) ? '取消' : '添加') + entry.emoji + '回应'" @click.stop="toggleReaction(item, entry.emoji)">{{ entry.emoji }} {{ entry.count }}</button></div></div><UserAvatar v-if="item.sender_id === me.id && item.kind !== 'system'" :user="me" :name="me.name" size="tiny" /></template>
            </div>
          </section>`)

// 4. member panel + member modal + mention list avatars
patch(`            <div v-for="person in currentMembers" :key="person.id" class="member-block-item" role="listitem">
              <span class="avatar tiny" :class="{ online: person.online }">{{ person.name.slice(0,1) }}</span>`,
  `            <div v-for="person in currentMembers" :key="person.id" class="member-block-item" role="listitem">
              <span class="avatar-wrap tiny" :class="{ online: person.online }"><UserAvatar :user="person" :name="person.alias || person.name" size="tiny" /></span>`)
patch(`<div v-for="person in currentMembers" :key="person.id" class="member-item" role="listitem"><span class="avatar tiny">{{ person.name.slice(0,1) }}</span>`,
  `<div v-for="person in currentMembers" :key="person.id" class="member-item" role="listitem"><UserAvatar :user="person" :name="person.alias || person.name" size="tiny" />`)
patch(`<button v-for="(person, index) in mentionList" :key="person.id" type="button" role="option" :aria-selected="index === mentionIndex" :class="{ active: index === mentionIndex }" @mousedown.prevent="chooseMention(person)"><span class="avatar tiny">{{ person.name.slice(0,1) }}</span>`,
  `<button v-for="(person, index) in mentionList" :key="person.id" type="button" role="option" :aria-selected="index === mentionIndex" :class="{ active: index === mentionIndex }" @mousedown.prevent="chooseMention(person)"><UserAvatar :user="person" :name="person.name" size="tiny" />`)

// 5. contacts modal + friend requests avatars
patch(`<div class="request-list"><div v-for="person in sortedFriends" :key="person.id" class="request-item"><div class="request-main"><strong>{{ person.remark || person.name }}</strong>`,
  `<div class="request-list"><div v-for="person in sortedFriends" :key="person.id" class="request-item request-with-avatar"><UserAvatar :user="person" :name="person.remark || person.name" /><div class="request-main"><strong>{{ person.remark || person.name }}</strong>`)
patch(`<div class="request-list"><div v-for="person in blockedUsers" :key="person.id" class="request-item"><div class="request-main"><strong>{{ person.name }}</strong>`,
  `<div class="request-list"><div v-for="person in blockedUsers" :key="person.id" class="request-item request-with-avatar"><UserAvatar :user="person" :name="person.name" /><div class="request-main"><strong>{{ person.name }}</strong>`)
patch(`<div v-for="item in friendRequests.incoming" :key="item.id" class="request-item"><div class="request-main"><strong>{{ item.requester.name }}</strong>`,
  `<div v-for="item in friendRequests.incoming" :key="item.id" class="request-item request-with-avatar"><UserAvatar :user="item.requester" :name="item.requester.name" /><div class="request-main"><strong>{{ item.requester.name }}</strong>`)
patch(`<div v-for="item in friendRequests.outgoing" :key="item.id" class="request-item"><div class="request-main"><strong>{{ item.recipient.name }}</strong>`,
  `<div v-for="item in friendRequests.outgoing" :key="item.id" class="request-item request-with-avatar"><UserAvatar :user="item.recipient" :name="item.recipient.name" /><div class="request-main"><strong>{{ item.recipient.name }}</strong>`)

// 6. thread window avatars
patch(`<div v-for="reply in threadReplies" :key="reply.id" class="thread-window-item" :class="{ mine: reply.sender_id === me.id, 'message-selected': messageActions.selectedIDs.includes(reply.id) }" @contextmenu.prevent.stop="messageActions.openMenu($event, reply)" @click.capture="messageActions.onMessageClick($event, reply)"><span class="avatar tiny">{{ sender(reply.sender_id).slice(0,1) }}</span>`,
  `<div class="thread-window-item" :class="{ mine: reply.sender_id === me.id, 'message-selected': messageActions.selectedIDs.includes(reply.id) }" @contextmenu.prevent.stop="messageActions.openMenu($event, reply)" @click.capture="messageActions.onMessageClick($event, reply)"><UserAvatar :user="memberByID(reply.sender_id)" :name="sender(reply.sender_id)" size="tiny" />`)

// 7. poke in the message context menu
patch(`        <button type="button" role="menuitem" @click="messageActions.runMenu('forward')">转发</button>`,
  `        <button type="button" role="menuitem" @click="messageActions.runMenu('forward')">转发</button>
        <button v-if="messageActions.menuMessage?.sender_id !== me.id" type="button" role="menuitem" @click="pokeMember(messageActions.menuMessage.sender_id); messageActions.closeMenu()">拍一拍</button>`)

// 8. direct info panel after the group panel
patch(`      <section v-if="threadOpen" ref="threadStream" class="thread-window"`,
  `      <aside v-if="current && current.kind === 'direct' && chatInfoOpen" class="detail-panel direct-panel" aria-label="会话信息">
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
      <section v-if="threadOpen" ref="threadStream" class="thread-window"`)

fs.writeFileSync(path, src)
console.log('template.js round2 patched')
