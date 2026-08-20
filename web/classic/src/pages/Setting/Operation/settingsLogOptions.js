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

export function normalizeLogSettingsValue(defaultValue, optionValue) {
  if (typeof defaultValue === 'boolean') {
    return (
      optionValue === true || optionValue === 'true' || optionValue === '1'
    );
  }
  if (typeof defaultValue === 'number') {
    const numberValue = Number(optionValue);
    return Number.isFinite(numberValue) ? numberValue : defaultValue;
  }
  return optionValue;
}

export function buildLogSettingsInputs(defaultInputs, options) {
  const nextInputs = { ...defaultInputs };
  for (let key in options || {}) {
    if (Object.prototype.hasOwnProperty.call(defaultInputs, key)) {
      nextInputs[key] = normalizeLogSettingsValue(
        defaultInputs[key],
        options[key],
      );
    }
  }
  return nextInputs;
}
