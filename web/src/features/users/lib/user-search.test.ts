/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.
*/
import { describe, expect, test } from 'vitest'

import { USER_ROLE } from '../constants'
import {
  canDemoteRootUser,
  canManageUserRoles,
  matchesUserSearchFilter,
} from './user-search'

const user = {
  id: 6281,
  username: 'mangolyw-cpu',
  display_name: 'Target User',
  email: 'target@example.com',
}

describe('user search contracts', () => {
  test('matches numeric IDs and text fields', () => {
    expect(matchesUserSearchFilter(user, '6281')).toBe(true)
    expect(matchesUserSearchFilter(user, 'mangolyw')).toBe(true)
    expect(matchesUserSearchFilter(user, 'target@')).toBe(true)
    expect(matchesUserSearchFilter(user, '7001')).toBe(false)
  })
})

describe('root role guards', () => {
  test('allows only root users to manage roles', () => {
    expect(canManageUserRoles(USER_ROLE.ROOT)).toBe(true)
    expect(canManageUserRoles(USER_ROLE.ADMIN)).toBe(false)
    expect(canManageUserRoles(USER_ROLE.USER)).toBe(false)
  })

  test('never offers self-demotion for a root account', () => {
    expect(canDemoteRootUser(10, { id: 10, role: USER_ROLE.ROOT })).toBe(false)
    expect(canDemoteRootUser(10, { id: 11, role: USER_ROLE.ROOT })).toBe(true)
    expect(canDemoteRootUser(10, { id: 11, role: USER_ROLE.ADMIN })).toBe(false)
  })
})
