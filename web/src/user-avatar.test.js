import test from 'node:test'
import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import { UserAvatar } from './user-avatar.js'

const avatarTemplate = UserAvatar.template
const defaultAvatar = readFileSync(new URL('../public/default-avatar.svg', import.meta.url), 'utf8')

test('users without a custom avatar use the built-in portrait', () => {
  assert.match(avatarTemplate, /url \|\| '\/default-avatar\.svg'/)
  assert.match(defaultAvatar, /<svg\b/)
  assert.match(defaultAvatar, /viewBox="0 0 128 128"/)
})

test('a custom avatar URL takes precedence over the default', () => {
  const bindings = UserAvatar.setup({ user: { avatar: 'avatar-id' } })
  assert.equal(bindings.url.value, '/api/v1/media/avatar-id')
  assert.match(avatarTemplate, /:alt="url \? '' : '默认头像'/)
})
