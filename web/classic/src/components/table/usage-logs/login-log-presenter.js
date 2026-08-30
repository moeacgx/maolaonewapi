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

export const LOG_TYPE_LOGIN = 7;

function readText(value) {
  return typeof value === 'string' ? value.trim() : '';
}

export function getLoginMethodFromLog(log, other) {
  const methodFromOperation = readText(other?.op?.params?.method);
  if (methodFromOperation) {
    return methodFromOperation;
  }

  const methodFromOther = readText(other?.login_method);
  if (methodFromOther) {
    return methodFromOther;
  }

  const match = readText(log?.content).match(
    /^Logged in successfully via\s+(.+)$/i,
  );
  return match ? match[1].trim() : '';
}

export function getLoginMethodLabel(method, t) {
  const normalized = readText(method);
  switch (normalized) {
    case 'password':
      return t('密码');
    case '2fa':
      return t('两步验证');
    case 'passkey':
      return 'Passkey';
    case 'wechat':
      return t('微信');
    case 'telegram':
      return 'Telegram';
    case 'oauth':
      return 'OAuth';
    default:
      break;
  }

  if (normalized.startsWith('oauth:')) {
    const provider = normalized.slice('oauth:'.length).trim();
    return provider ? `OAuth ${provider}` : 'OAuth';
  }

  return normalized || t('未知');
}

export function getLoginLogSummary(log, other, t) {
  if (!log || log.type !== LOG_TYPE_LOGIN) {
    return null;
  }

  const method = getLoginMethodFromLog(log, other);
  if (method) {
    return t('登录成功（通过 {{method}}）', {
      method: getLoginMethodLabel(method, t),
    });
  }

  return readText(log.content) || t('登录成功');
}

export function getLoginLogDetailItems(log, other, t) {
  if (!log || log.type !== LOG_TYPE_LOGIN) {
    return [];
  }

  const method = getLoginMethodFromLog(log, other);
  const summary = getLoginLogSummary(log, other, t);
  const items = [];

  if (summary) {
    items.push({ key: t('登录信息'), value: summary });
  }
  if (method) {
    items.push({ key: t('登录方式'), value: getLoginMethodLabel(method, t) });
  }
  if (readText(log.ip)) {
    items.push({ key: t('IP'), value: readText(log.ip) });
  }
  if (readText(other?.user_agent)) {
    items.push({ key: t('User Agent'), value: readText(other.user_agent) });
  }

  return items;
}
