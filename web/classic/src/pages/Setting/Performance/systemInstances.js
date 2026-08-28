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

export const SYSTEM_INSTANCE_POLL_INTERVAL_MS = 30000;

export function getSystemInstancesFromResponse(response) {
  const instances = response?.data?.data;
  return Array.isArray(instances) ? instances : [];
}

export function getInstanceDisplayName(instance) {
  return instance?.info?.node?.name || instance?.node_name || '-';
}

export function getInstanceHostname(instance) {
  return instance?.info?.host?.hostname || '-';
}

export function shouldConfigureNodeName(instance) {
  return instance?.info?.node?.should_configure_manually === true;
}

export function isStaleInstance(instance) {
  return instance?.status === 'stale';
}

export function getInstanceStatusLabel(status) {
  if (status === 'online') return 'Online';
  if (status === 'stale') return 'Stale';
  return status || '-';
}

export function getInstanceStatusTagColor(status) {
  if (status === 'online') return 'green';
  if (status === 'stale') return 'orange';
  return 'grey';
}

export function getInstanceRoleLabel(instance) {
  return instance?.info?.role?.is_master ? 'master' : 'worker';
}

export function getInstanceRoleDescription(instance) {
  if (instance?.info?.role?.is_master) {
    return 'Master instances run scheduled background tasks.';
  }
  return 'Worker instances do not run master-only background tasks.';
}

export function getInstanceRuntimeLabel(instance) {
  const runtime = instance?.info?.runtime;
  const values = [runtime?.goos, runtime?.goarch].filter(Boolean);
  return values.length ? values.join('/') : '-';
}

export function normalizePercent(value) {
  if (typeof value !== 'number' || Number.isNaN(value)) return null;
  return Math.max(0, Math.min(100, value));
}

export function formatPercent(value) {
  if (typeof value !== 'number' || Number.isNaN(value)) return '-';
  return `${Number(value.toFixed(1))}%`;
}

export function getResourceColor(percent) {
  if (percent === null) return 'var(--semi-color-disabled-text)';
  if (percent >= 90) return 'var(--semi-color-danger)';
  if (percent >= 70) return 'var(--semi-color-warning)';
  return 'var(--semi-color-success)';
}

export function formatBytes(bytes, decimals = 1) {
  if (typeof bytes !== 'number' || Number.isNaN(bytes)) return '-';
  if (bytes === 0) return '0 Bytes';
  if (bytes < 0) return `-${formatBytes(-bytes, decimals)}`;

  const base = 1024;
  const units = ['Bytes', 'KB', 'MB', 'GB', 'TB'];
  const index = Math.min(
    Math.floor(Math.log(bytes) / Math.log(base)),
    units.length - 1,
  );
  const value = bytes / Math.pow(base, index);
  return `${Number(value.toFixed(index === 0 ? 0 : decimals))} ${units[index]}`;
}
