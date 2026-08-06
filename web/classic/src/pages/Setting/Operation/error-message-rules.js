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

export const ERROR_MESSAGE_REPLACEMENT_MODES = ['contains', 'exact', 'regex'];

export const MAX_ERROR_MESSAGE_REPLACEMENT_RULES = 100;
export const MAX_ERROR_MESSAGE_MATCHES_PER_RULE = 64;
export const MAX_ERROR_MESSAGE_MATCH_LENGTH = 4096;
export const MAX_ERROR_MESSAGE_REPLACE_LENGTH = 4096;

const isValidStatusCode = (value) =>
  value === undefined ||
  (Number.isInteger(value) && value >= 100 && value <= 599);

const parseStatusCode = (value) =>
  isValidStatusCode(value) ? value : undefined;

const parseMatches = (rule) => {
  if (Array.isArray(rule.matches)) {
    return rule.matches.every((value) => typeof value === 'string')
      ? rule.matches
      : [];
  }
  return typeof rule.match === 'string' ? [rule.match] : [];
};

export function parseErrorMessageReplacementRules(raw) {
  try {
    const value = JSON.parse(raw || '[]');
    if (!Array.isArray(value)) return [];
    return value
      .filter(
        (rule) =>
          rule &&
          typeof rule.replace === 'string' &&
          ERROR_MESSAGE_REPLACEMENT_MODES.includes(rule.mode),
      )
      .slice(0, MAX_ERROR_MESSAGE_REPLACEMENT_RULES)
      .map((rule) => ({
        matches: parseMatches(rule),
        mode: rule.mode,
        status_code: parseStatusCode(rule.status_code),
        replace: rule.replace,
        replace_status_code: parseStatusCode(rule.replace_status_code),
      }));
  } catch {
    return [];
  }
}

export function serializeErrorMessageReplacementRules(rules) {
  return JSON.stringify(
    rules.map((rule) => ({
      match: rule.matches[0]?.trim(),
      matches: rule.matches.map((match) => match.trim()),
      mode: rule.mode,
      status_code: rule.status_code,
      replace: rule.replace.trim(),
      replace_status_code: rule.replace_status_code,
    })),
  );
}

export function createErrorMessageReplacementRule() {
  return {
    matches: [''],
    mode: 'contains',
    status_code: undefined,
    replace: '',
    replace_status_code: undefined,
  };
}

export function validateErrorMessageReplacementRules(rules) {
  return (
    rules.length <= MAX_ERROR_MESSAGE_REPLACEMENT_RULES &&
    rules.every(
      (rule) =>
        Array.isArray(rule.matches) &&
        rule.matches.length > 0 &&
        rule.matches.length <= MAX_ERROR_MESSAGE_MATCHES_PER_RULE &&
        rule.matches.every(
          (match) =>
            typeof match === 'string' &&
            match.trim().length > 0 &&
            Array.from(match.trim()).length <= MAX_ERROR_MESSAGE_MATCH_LENGTH,
        ) &&
        typeof rule.replace === 'string' &&
        rule.replace.trim().length > 0 &&
        Array.from(rule.replace.trim()).length <=
          MAX_ERROR_MESSAGE_REPLACE_LENGTH &&
        ERROR_MESSAGE_REPLACEMENT_MODES.includes(rule.mode) &&
        isValidStatusCode(rule.status_code) &&
        isValidStatusCode(rule.replace_status_code),
    )
  );
}
