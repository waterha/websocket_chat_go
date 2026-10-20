// Group governance helpers. Kept separate so the permission and role rules that
// the UI shows can be unit tested without rendering the whole chat component.

export const ROLE_LABELS = { owner: '群主', admin: '管理员', member: '成员' }

export const roleLabel = role => ROLE_LABELS[role] || ROLE_LABELS.member

export function roleOf(conversation, userID) {
  return (conversation?.members || []).find(member => member.id === userID)?.role || 'member'
}

// Any group member may invite others, matching the documented behaviour; the
// destructive governance actions below are what roles actually gate.
export function canInvite(conversation, userID) {
  if (conversation?.kind !== 'group') return false
  return (conversation.members || []).some(member => member.id === userID)
}

// Removing others and editing the announcement are reserved for owner/admin.
export function canManageMembers(conversation, userID) {
  if (conversation?.kind !== 'group') return false
  const role = roleOf(conversation, userID)
  return role === 'owner' || role === 'admin'
}

export function canChangeRole(conversation, userID) {
  return conversation?.kind === 'group' && roleOf(conversation, userID) === 'owner'
}

export function canTransferOwnership(conversation, userID) {
  return canChangeRole(conversation, userID)
}

export function canEditAnnouncement(conversation, userID) {
  return canManageMembers(conversation, userID)
}

// A member can always leave; only a non-owner may be removed by someone else.
export function canLeave(conversation) {
  return conversation?.kind === 'group'
}

export function canRemoveMember(conversation, userID, targetID) {
  if (conversation?.kind !== 'group' || !targetID || targetID === userID) return false
  const caller = roleOf(conversation, userID)
  const target = roleOf(conversation, targetID)
  if (target === 'owner') return false
  if (caller === 'owner') return true
  return caller === 'admin' && target === 'member'
}
