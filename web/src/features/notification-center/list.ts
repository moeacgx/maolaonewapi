/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program. If not, see <https://www.gnu.org/licenses/>.

For commercial licensing, please contact support@quantumnous.com
*/

export type NotificationListPayload<T> = T[] | { items?: T[] }

/** Normalizes both list response shapes supported by the notification API. */
export function normalizeNotificationList<T>(
  payload?: NotificationListPayload<T>
): T[] {
  if (Array.isArray(payload)) return payload
  return payload?.items ?? []
}

/** The center intentionally shows only the five most recent deliveries. */
export function takeRecentNotifications<T>(items: readonly T[]): T[] {
  return items.slice(0, 5)
}
