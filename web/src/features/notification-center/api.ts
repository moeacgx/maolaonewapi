/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.
*/
import { api } from '@/lib/api'

import {
  normalizeNotificationList,
  takeRecentNotifications,
  type NotificationListPayload,
} from './list'
import type {
  NotificationApiResponse,
  NotificationBot,
  NotificationBotInput,
  NotificationDelivery,
  NotificationEventType,
  NotificationTask,
  NotificationTaskInput,
} from './types'

function requireSuccessfulResponse<T>(response: NotificationApiResponse<T>) {
  if (response.success === false) {
    throw new Error(response.message || 'Request failed')
  }
  return response
}

export async function getNotificationBots(): Promise<NotificationBot[]> {
  const response = await api.get<
    NotificationApiResponse<NotificationListPayload<NotificationBot>>
  >('/api/notification/bots')
  return normalizeNotificationList(
    requireSuccessfulResponse(response.data).data
  )
}

export async function createNotificationBot(
  input: NotificationBotInput
): Promise<NotificationApiResponse<NotificationBot>> {
  const response = await api.post('/api/notification/bots', input)
  return response.data
}

export async function updateNotificationBot(
  id: number,
  input: NotificationBotInput
): Promise<NotificationApiResponse<NotificationBot>> {
  const response = await api.put(`/api/notification/bots/${id}`, input)
  return response.data
}

export async function deleteNotificationBot(
  id: number
): Promise<NotificationApiResponse> {
  const response = await api.delete(`/api/notification/bots/${id}`)
  return response.data
}

export async function testNotificationBot(
  id: number,
  chatId: string
): Promise<NotificationApiResponse> {
  const response = await api.post(`/api/notification/bots/${id}/test`, {
    chat_id: chatId,
  })
  return response.data
}

export async function getNotificationTasks(): Promise<NotificationTask[]> {
  const response = await api.get<
    NotificationApiResponse<NotificationListPayload<NotificationTask>>
  >('/api/notification/tasks')
  return normalizeNotificationList(
    requireSuccessfulResponse(response.data).data
  )
}

export async function createNotificationTask(
  input: NotificationTaskInput
): Promise<NotificationApiResponse<number>> {
  const response = await api.post('/api/notification/tasks', input)
  return response.data
}

export async function updateNotificationTask(
  id: number,
  input: NotificationTaskInput
): Promise<NotificationApiResponse<null>> {
  const response = await api.put(`/api/notification/tasks/${id}`, input)
  return response.data
}

export async function deleteNotificationTask(
  id: number
): Promise<NotificationApiResponse> {
  const response = await api.delete(`/api/notification/tasks/${id}`)
  return response.data
}

export async function getNotificationEventTypes(): Promise<
  NotificationEventType[]
> {
  const response = await api.get<
    NotificationApiResponse<NotificationListPayload<NotificationEventType>>
  >('/api/notification/event-types')
  return normalizeNotificationList(
    requireSuccessfulResponse(response.data).data
  )
}

export async function getNotificationDeliveries(): Promise<
  NotificationDelivery[]
> {
  const response = await api.get<
    NotificationApiResponse<NotificationListPayload<NotificationDelivery>>
  >('/api/notification/deliveries')
  return takeRecentNotifications(
    normalizeNotificationList(requireSuccessfulResponse(response.data).data)
  )
}
