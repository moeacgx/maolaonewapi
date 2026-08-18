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

// 将新会话认证包转换为 Classic 面板仍使用的用户对象形状。
export function normalizeAuthData(data) {
  if (!data || typeof data !== 'object') return data;
  const user = data.user && typeof data.user === 'object' ? data.user : data;
  const accessToken = data.access_token || data.token || user.token;
  return {
    ...user,
    ...(accessToken ? { token: accessToken, access_token: accessToken } : {}),
    ...(data.token_type ? { token_type: data.token_type } : {}),
    ...(data.access_expires_at
      ? { access_expires_at: data.access_expires_at }
      : {}),
    ...(data.session ? { session: data.session } : {}),
  };
}
