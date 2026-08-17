/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.
*/
import { USER_ROLE } from '../constants'
import type { User } from '../types'

type SearchableUser = Pick<User, 'id' | 'username' | 'display_name' | 'email'>

export function matchesUserSearchFilter(
  user: SearchableUser,
  filterValue: unknown
): boolean {
  const searchValue = String(filterValue).trim().toLowerCase()
  if (!searchValue) return true
  return [user.id, user.username, user.display_name, user.email].some((field) =>
    String(field || '')
      .toLowerCase()
      .includes(searchValue)
  )
}

export function canManageUserRoles(currentUserRole?: number): boolean {
  return currentUserRole === USER_ROLE.ROOT
}

export function canDemoteRootUser(
  currentUserId: number | undefined,
  target: Pick<User, 'id' | 'role'>
): boolean {
  return target.role === USER_ROLE.ROOT && target.id !== currentUserId
}
