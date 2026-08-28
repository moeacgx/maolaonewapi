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

export function buildChannelAffinityUsageCacheTarget(affinity) {
  const value = affinity || {};
  return {
    rule_name: value.rule_name || value.reason || '',
    using_group: value.using_group || '',
    key_hint: value.key_hint || '',
    key_fp: value.key_fp || '',
  };
}

export function hasChannelAffinityUsageCacheIdentity(target) {
  return Boolean(
    target &&
      String(target.rule_name || '').trim() &&
      String(target.key_fp || '').trim(),
  );
}

export function hasChannelAffinityUsageCacheMetric(
  stats,
  field,
  supportsTokenStats,
) {
  return Boolean(
    supportsTokenStats &&
      stats &&
      Object.prototype.hasOwnProperty.call(stats, field),
  );
}
