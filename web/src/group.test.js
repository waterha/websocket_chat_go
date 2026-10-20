import test from 'node:test'
import assert from 'node:assert/strict'
import { canChangeRole, canEditAnnouncement, canInvite, canLeave, canManageMembers, canRemoveMember, canTransferOwnership, roleLabel, roleOf } from './group.js'

const group = (roles) => ({
  kind: 'group',
  members: Object.entries(roles).map(([id, role]) => ({ id, name: id, role })),
})

const owner = group({ alice: 'owner', bob: 'admin', carol: 'member' })
const direct = { kind: 'direct', members: [{ id: 'alice', role: 'member' }, { id: 'bob', role: 'member' }] }

test('roles are read from the conversation, defaulting to member', () => {
  assert.equal(roleOf(owner, 'alice'), 'owner')
  assert.equal(roleOf(owner, 'bob'), 'admin')
  assert.equal(roleOf(owner, 'carol'), 'member')
  assert.equal(roleOf(owner, 'unknown'), 'member')
  assert.equal(roleOf(null, 'alice'), 'member')
  assert.equal(roleLabel('owner'), '群主')
  assert.equal(roleLabel('admin'), '管理员')
  assert.equal(roleLabel('member'), '成员')
  assert.equal(roleLabel('nonsense'), '成员')
})

test('any group member may invite; announcement editing needs owner or admin', () => {
  assert.equal(canInvite(owner, 'alice'), true)
  assert.equal(canInvite(owner, 'bob'), true)
  assert.equal(canInvite(owner, 'carol'), true)
  assert.equal(canManageMembers(owner, 'carol'), false)
  assert.equal(canManageMembers(owner, 'bob'), true)
  assert.equal(canEditAnnouncement(owner, 'carol'), false)
  assert.equal(canEditAnnouncement(owner, 'bob'), true)
  assert.equal(canInvite(direct, 'alice'), false)
  assert.equal(canInvite(null, 'alice'), false)
})

test('only the owner changes roles or transfers ownership', () => {
  assert.equal(canChangeRole(owner, 'alice'), true)
  assert.equal(canChangeRole(owner, 'bob'), false)
  assert.equal(canTransferOwnership(owner, 'alice'), true)
  assert.equal(canTransferOwnership(owner, 'bob'), false)
  assert.equal(canChangeRole(direct, 'alice'), false)
})

test('removal rules match the server: nobody removes the owner, admins only remove members', () => {
  assert.equal(canRemoveMember(owner, 'alice', 'carol'), true)
  assert.equal(canRemoveMember(owner, 'alice', 'bob'), true)
  assert.equal(canRemoveMember(owner, 'alice', 'alice'), false)
  assert.equal(canRemoveMember(owner, 'bob', 'carol'), true)
  assert.equal(canRemoveMember(owner, 'bob', 'alice'), false)
  assert.equal(canRemoveMember(owner, 'bob', 'bob'), false)
  assert.equal(canRemoveMember(owner, 'carol', 'bob'), false)
  assert.equal(canRemoveMember(direct, 'alice', 'bob'), false)
})

test('any group member may leave', () => {
  assert.equal(canLeave(owner), true)
  assert.equal(canLeave(direct), false)
  assert.equal(canLeave(null), false)
})
