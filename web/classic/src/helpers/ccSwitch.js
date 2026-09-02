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

export function normalizeCCSwitchAddress(value) {
  return String(value || '')
    .trim()
    .replace(/\/+$/, '');
}

export function withCCSwitchAPIAddress(status, value) {
  const current =
    status && typeof status === 'object' && !Array.isArray(status)
      ? status
      : {};
  const normalized = normalizeCCSwitchAddress(value);
  const next = {
    ...current,
    cc_switch_api_address: normalized,
  };

  if (current.data && typeof current.data === 'object') {
    next.data = {
      ...current.data,
      cc_switch_api_address: normalized,
    };
  }

  return next;
}

export function syncCCSwitchAPIAddressToStorage(value, storage) {
  let current = {};
  try {
    const raw = storage.getItem('status');
    current = raw ? JSON.parse(raw) : {};
  } catch (_) {}

  const next = withCCSwitchAPIAddress(current, value);
  storage.setItem('status', JSON.stringify(next));
  return next;
}

export function isValidCCSwitchAddress(value) {
  const address = normalizeCCSwitchAddress(value);
  if (!address) return true;

  try {
    const parsed = new URL(address);
    return (
      ['http:', 'https:'].includes(parsed.protocol) &&
      !parsed.username &&
      !parsed.password &&
      !parsed.search &&
      !parsed.hash
    );
  } catch (_) {
    return false;
  }
}

export function isCCSwitchPreset(value) {
  return String(value || '')
    .trim()
    .toLowerCase()
    .startsWith('ccswitch');
}

export function resolveCCSwitchAddresses(status, origin) {
  const fallbackAddress = normalizeCCSwitchAddress(origin);
  const homepage =
    normalizeCCSwitchAddress(status?.server_address) || fallbackAddress;
  const apiAddress =
    normalizeCCSwitchAddress(status?.cc_switch_api_address) || homepage;

  return { apiAddress, homepage };
}

export function buildCCSwitchURL({
  app,
  name,
  models,
  apiKey,
  status,
  origin,
}) {
  const { apiAddress, homepage } = resolveCCSwitchAddresses(status, origin);
  const endpoint =
    app === 'codex' && !/\/v1$/i.test(apiAddress)
      ? `${apiAddress}/v1`
      : apiAddress;
  const params = new URLSearchParams();
  params.set('resource', 'provider');
  params.set('app', app);
  params.set('name', name);
  params.set('endpoint', endpoint);
  params.set('apiKey', apiKey);
  if (app === 'codex') {
    params.set('supports_websockets', 'true');
  }
  for (const [key, value] of Object.entries(models)) {
    if (value) params.set(key, value);
  }
  params.set('homepage', homepage);
  params.set('enabled', 'true');
  return `ccswitch://v1/import?${params.toString()}`;
}
