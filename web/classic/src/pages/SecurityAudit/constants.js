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

export const SECURITY_AUDIT_SCANNERS = [
  { value: 'violent', label: '暴力内容' },
  { value: 'non_violent_illegal_acts', label: '非暴力违法行为' },
  { value: 'sexual_content_or_sexual_acts', label: '色情内容或性行为' },
  { value: 'pii', label: '个人敏感信息' },
  { value: 'suicide_and_self_harm', label: '自杀与自残' },
  { value: 'unethical_acts', label: '不道德行为' },
  { value: 'politically_sensitive_topics', label: '政治敏感话题' },
  { value: 'copyright_violation', label: '版权侵权' },
  { value: 'jailbreak', label: '越狱攻击' },
];

export const MODE_OPTIONS = [
  { value: 'off', label: '关闭' },
  { value: 'async_audit', label: '异步审计' },
  { value: 'blocking', label: '同步阻断' },
];

export const getModeLabel = (mode, t) =>
  t(MODE_OPTIONS.find((item) => item.value === mode)?.label || '关闭');

export const getDecisionColor = (decision) => {
  switch (String(decision || '').toLowerCase()) {
    case 'pass':
    case 'allow':
    case 'allowed':
    case 'safe':
      return 'green';
    case 'critical':
    case 'blocked':
    case 'block':
      return 'red';
    case 'flag':
    case 'flagged':
      return 'orange';
    default:
      return 'grey';
  }
};

export const getRiskColor = (risk) => {
  switch (String(risk || '').toLowerCase()) {
    case 'high':
    case 'critical':
      return 'red';
    case 'medium':
      return 'orange';
    case 'low':
      return 'yellow';
    case 'safe':
      return 'green';
    default:
      return 'grey';
  }
};
