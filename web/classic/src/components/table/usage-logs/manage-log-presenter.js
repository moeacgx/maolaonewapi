export function getManageLogSummary(log, other, t) {
  if (log?.type !== 3 || !other?.op?.action) return null;

  const params = other.op.params || {};
  const quota = params.quota || '';
  const summaries = {
    'user.quota_add': `${t('管理员增加用户额度')} ${quota}`,
    'user.quota_subtract': `${t('管理员减少用户额度')} ${quota}`,
    'user.quota_override': `${t('管理员覆盖用户额度')} ${params.from || ''} ${t('为')} ${params.to || ''}`,
  };
  return summaries[other.op.action] || null;
}
