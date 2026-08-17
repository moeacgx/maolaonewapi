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

export const cleanSecurityAuditFilter = (filter = {}) =>
  Object.fromEntries(
    Object.entries(filter).flatMap(([key, value]) => {
      if (typeof value === 'string') {
        const normalized = value.trim();
        return normalized ? [[key, normalized]] : [];
      }
      if (typeof value === 'number') {
        return Number.isFinite(value) && value > 0 ? [[key, value]] : [];
      }
      return value !== null && value !== undefined ? [[key, value]] : [];
    }),
  );
