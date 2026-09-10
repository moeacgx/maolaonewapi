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

// 编辑转换同时检查凭据的来源类型和目标类型。
export function canConvertChannelToMultiKey({
  isEdit,
  isMultiKeyChannel,
  originalType,
  type,
  isIonetLocked,
}) {
  return Boolean(
    isEdit &&
      !isMultiKeyChannel &&
      !isIonetLocked &&
      originalType &&
      ![41, 57].includes(originalType) &&
      ![41, 57].includes(type),
  );
}

// 覆盖表单残留值，防止取消转换后仍提交密钥模式。
export function buildChannelKeyUpdateFields({
  isMultiKeyChannel = false,
  convertToMultiKey = false,
  keyMode = 'append',
  multiKeyMode = 'random',
}) {
  const enabled = isMultiKeyChannel || convertToMultiKey;
  return {
    convert_to_multi_key: convertToMultiKey || undefined,
    key_mode: enabled ? keyMode : undefined,
    multi_key_mode: enabled ? multiKeyMode : undefined,
  };
}
