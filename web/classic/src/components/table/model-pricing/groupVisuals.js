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

const GROUP_SEMANTIC_COLORS = [
  'amber',
  'blue',
  'cyan',
  'green',
  'grey',
  'indigo',
  'light-blue',
  'lime',
  'orange',
  'pink',
  'purple',
  'red',
  'teal',
  'violet',
  'yellow',
];

const GROUP_TEXT_COLOR_BY_SEMANTIC_COLOR = {
  amber: '#b7791f',
  blue: 'var(--semi-color-primary)',
  cyan: '#0891b2',
  green: 'var(--semi-color-success)',
  grey: 'var(--semi-color-text-2)',
  indigo: '#4f46e5',
  'light-blue': '#0284c7',
  lime: '#65a30d',
  orange: '#ea580c',
  pink: '#db2777',
  purple: '#7c3aed',
  red: 'var(--semi-color-danger)',
  teal: '#0d9488',
  violet: '#8b5cf6',
  yellow: '#ca8a04',
};

export const getGroupSemanticColor = (group) => {
  const normalizedGroup = String(group ?? '').trim();
  if (!normalizedGroup) return 'grey';

  let sum = 0;
  for (let index = 0; index < normalizedGroup.length; index += 1) {
    sum += normalizedGroup.charCodeAt(index);
  }
  return GROUP_SEMANTIC_COLORS[sum % GROUP_SEMANTIC_COLORS.length];
};

export const getGroupTextColor = (group) =>
  GROUP_TEXT_COLOR_BY_SEMANTIC_COLOR[getGroupSemanticColor(group)] ||
  'var(--semi-color-primary)';
