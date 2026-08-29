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

// 只发送渠道禁用事件支持的筛选字段，并按后端匹配语义清理重复或空关键词。
export const normalizeNotificationFilterConfig = (config) => {
  if (!config) return undefined;

  const statusCodes = String(config.status_codes || '').trim();
  const errorKeywords = [];
  const seenKeywords = new Set();
  for (const value of Array.isArray(config.error_keywords)
    ? config.error_keywords
    : []) {
    const keyword = String(value || '').trim();
    const identity = keyword.toLocaleLowerCase();
    if (!keyword || seenKeywords.has(identity)) continue;
    seenKeywords.add(identity);
    errorKeywords.push(keyword);
  }

  if (!statusCodes && errorKeywords.length === 0) return undefined;

  return {
    ...(statusCodes ? { status_codes: statusCodes } : {}),
    ...(errorKeywords.length > 0 ? { error_keywords: errorKeywords } : {}),
  };
};
