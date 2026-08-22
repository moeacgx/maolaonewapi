/*
Copyright (C) 2025 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as published by
the Free Software Foundation, either version 3 of the License, or (at your
option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program. If not, see <https://www.gnu.org/licenses/>.
*/

export const TOPUP_QUOTA_LIMIT_ERROR = 'top-up quota limit exceeded';
export const TOPUP_QUOTA_LIMIT_MESSAGE =
  'Top-up would exceed the wallet quota limit. Please reduce the amount or contact an administrator.';

export function getTopupErrorMessage(
  message,
  data,
  translate,
  fallback = '支付请求失败',
) {
  const dataMessage = typeof data === 'string' ? data.trim() : '';
  const responseMessage = typeof message === 'string' ? message.trim() : '';
  const rawMessage = dataMessage || responseMessage;

  if (rawMessage === TOPUP_QUOTA_LIMIT_ERROR) {
    return translate(TOPUP_QUOTA_LIMIT_MESSAGE);
  }

  if (!rawMessage || rawMessage === 'error') {
    return translate(fallback);
  }

  return rawMessage;
}
