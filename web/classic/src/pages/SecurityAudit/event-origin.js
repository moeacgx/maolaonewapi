/*
Copyright (C) 2025 QuantumNous

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

export const AUDIT_EVENT_ORIGIN_ASSIGNED = 'assigned';
export const AUDIT_EVENT_ORIGIN_UNASSIGNED = 'unassigned';
export const AUDIT_EVENT_ORIGIN_HISTORICAL = 'historical';

const hasOwn = (value, key) =>
  value != null && Object.prototype.hasOwnProperty.call(value, key);

const positiveId = (value) => {
  const id = Number(value);
  return Number.isInteger(id) && id > 0 ? id : 0;
};

const cleanText = (value) => String(value || '').trim();

const isRequestSideStage = (stage) => {
  const normalized = cleanText(stage).toLowerCase();
  return !normalized.includes('response');
};

export const getAuditEventChannelOrigin = (event = {}) => {
  const hasSnapshot =
    hasOwn(event, 'channel_id') ||
    hasOwn(event, 'channel_name') ||
    hasOwn(event, 'channel_groups');
  if (!hasSnapshot) {
    return { state: AUDIT_EVENT_ORIGIN_HISTORICAL, id: 0, name: '' };
  }

  const id = positiveId(event.channel_id);
  if (id > 0) {
    return {
      state: AUDIT_EVENT_ORIGIN_ASSIGNED,
      id,
      name: cleanText(event.channel_name),
    };
  }

  return {
    state: isRequestSideStage(event.stage)
      ? AUDIT_EVENT_ORIGIN_UNASSIGNED
      : AUDIT_EVENT_ORIGIN_HISTORICAL,
    id: 0,
    name: '',
  };
};

const normalizeChannelGroups = (groups) => {
  if (!Array.isArray(groups)) return [];

  const seen = new Set();
  return groups.flatMap((group) => {
    const item = {
      id: positiveId(group?.id),
      code: cleanText(group?.code),
      name: cleanText(group?.name),
    };
    if (!item.id && !item.code && !item.name) return [];

    const identity = item.id
      ? `id:${item.id}`
      : `code:${item.code || item.name}`;
    if (seen.has(identity)) return [];
    seen.add(identity);
    return [item];
  });
};

export const getAuditEventRouteGroupOrigin = (event = {}) => {
  const routeGroup = {
    id: positiveId(event.group_id),
    code: '',
    name: cleanText(event.group_name),
  };
  if (routeGroup.id > 0 || routeGroup.name) {
    return { state: AUDIT_EVENT_ORIGIN_ASSIGNED, items: [routeGroup] };
  }

  const hasSnapshot =
    hasOwn(event, 'channel_groups') ||
    hasOwn(event, 'group_id') ||
    hasOwn(event, 'group_name');
  if (!hasSnapshot) {
    return { state: AUDIT_EVENT_ORIGIN_HISTORICAL, items: [] };
  }

  return {
    state: isRequestSideStage(event.stage)
      ? AUDIT_EVENT_ORIGIN_UNASSIGNED
      : AUDIT_EVENT_ORIGIN_HISTORICAL,
    items: [],
  };
};

export const getAuditEventChannelGroupsOrigin = (event = {}) => {
  const channelGroups = normalizeChannelGroups(event.channel_groups);
  if (channelGroups.length > 0) {
    return { state: AUDIT_EVENT_ORIGIN_ASSIGNED, items: channelGroups };
  }

  const channel = getAuditEventChannelOrigin(event);
  return {
    state: channel.state,
    items: [],
  };
};
