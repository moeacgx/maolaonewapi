const sdk = globalThis.__NEW_API_EXTENSION_NATIVE_SDK__;
if (!sdk || sdk.sdk !== 'v1' || sdk.platform !== 'default') {
  throw new Error('Native extension SDK v1 for default is unavailable.');
}

const React = sdk.modules.react;
const Runtime = sdk.modules['react/jsx-runtime'];
const Query = sdk.modules['@tanstack/react-query'];
const I18n = sdk.modules['react-i18next'];
const ApiModule = sdk.modules['@/lib/api'];
const Layout = sdk.modules['@/components/layout'];
const UI = {
  alert: sdk.modules['@/components/ui/alert'],
  badge: sdk.modules['@/components/ui/badge'],
  button: sdk.modules['@/components/ui/button'],
  card: sdk.modules['@/components/ui/card'],
  table: sdk.modules['@/components/ui/table'],
};
const Toast = sdk.modules.sonner;
if (!React || !Runtime || !Query || !I18n || !ApiModule || !Layout) {
  throw new Error('Required host SDK modules are unavailable.');
}

const { jsx, jsxs, Fragment } = Runtime;
const { useEffect, useMemo, useState } = React;
const { useQuery } = Query;
const { useTranslation } = I18n;
const { api } = ApiModule;
const { SectionPageLayout } = Layout;
const { Alert, AlertDescription, AlertTitle } = UI.alert;
const { Badge } = UI.badge;
const { Button } = UI.button;
const { Card, CardContent, CardDescription, CardHeader, CardTitle } = UI.card;
const { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } = UI.table;

const basePath = '/api/extensions/conversation-archive';

function unwrap(response) {
  const payload = response?.data ?? response;
  if (payload?.success === false) throw new Error(payload.message || 'Request failed');
  return payload?.data ?? payload;
}

async function loadConfig() {
  return unwrap(await api.get(`${basePath}/config`, { skipErrorHandler: true }));
}

async function loadGroups() {
  return unwrap(await api.get(`${basePath}/groups`, { skipErrorHandler: true }));
}

async function loadArchives(params) {
  return unwrap(await api.get(`${basePath}/conversations`, { params, skipErrorHandler: true }));
}

async function loadArchive(id) {
  return unwrap(await api.get(`${basePath}/conversations/${id}`, { skipErrorHandler: true }));
}

async function clearArchives() {
  return unwrap(await api.post(`${basePath}/conversations/clear`, { confirm: true }, { skipErrorHandler: true }));
}

function formatTime(value) {
  if (!value) return '-';
  return new Intl.DateTimeFormat(undefined, {
    year: 'numeric', month: '2-digit', day: '2-digit',
    hour: '2-digit', minute: '2-digit', second: '2-digit',
  }).format(new Date(Number(value) * 1000));
}

function formatBytes(value) {
  const size = Number(value || 0);
  if (size < 1024) return `${size} B`;
  if (size < 1024 * 1024) return `${(size / 1024).toFixed(1)} KiB`;
  return `${(size / 1024 / 1024).toFixed(2)} MiB`;
}

function parseUserIds(value) {
  return [...new Set(String(value || '').split(/[\s,，]+/).map((item) => Number(item)).filter((id) => Number.isInteger(id) && id > 0))];
}

function GroupPicker({ groups, values, onChange, label }) {
  return jsx('div', { className: 'archive-field', children: [
    jsx('label', { children: label }),
    jsx('select', {
      multiple: true,
      value: values,
      onChange: (event) => onChange([...event.target.selectedOptions].map((option) => option.value)),
      'aria-label': label,
      children: (groups || []).map((group) => jsx('option', {
        value: group.code,
        children: `${group.name || group.code} (${group.code})`,
      }, group.id || group.code)),
    }),
  ] });
}

function ConfigCard({ config, groups, refresh }) {
  const { t } = useTranslation();
  const [enabled, setEnabled] = useState(Boolean(config?.enabled));
  const [groupCodes, setGroupCodes] = useState(config?.group_codes || []);
  const [userIds, setUserIds] = useState((config?.user_ids || []).join(', '));
  const [retentionDays, setRetentionDays] = useState(String(config?.retention_days || 30));
  const [maxBodyBytes, setMaxBodyBytes] = useState(String(config?.max_body_bytes || 2097152));
  const [maxArchiveCount, setMaxArchiveCount] = useState(String(config?.max_archive_count || 1000));
  const [saving, setSaving] = useState(false);

  useEffect(() => {
    setEnabled(Boolean(config?.enabled));
    setGroupCodes(config?.group_codes || []);
    setUserIds((config?.user_ids || []).join(', '));
    setRetentionDays(String(config?.retention_days || 30));
    setMaxBodyBytes(String(config?.max_body_bytes || 2097152));
    setMaxArchiveCount(String(config?.max_archive_count || 1000));
  }, [config?.config_version]);

  async function save() {
    setSaving(true);
    try {
      const payload = {
        expected_version: Number(config?.config_version || 0),
        enabled,
        group_codes: groupCodes,
        user_ids: parseUserIds(userIds),
        retention_days: Number(retentionDays),
        max_body_bytes: Number(maxBodyBytes),
        max_archive_count: Number(maxArchiveCount),
      };
      unwrap(await api.put(`${basePath}/config`, payload, { skipErrorHandler: true }));
      Toast?.toast?.success?.(t('Conversation archive settings saved'));
      refresh();
    } catch (error) {
      Toast?.toast?.error?.(error instanceof Error ? error.message : t('Request failed'));
    } finally {
      setSaving(false);
    }
  }

  return jsxs(Card, { children: [
    jsxs(CardHeader, { children: [
      jsx(CardTitle, { children: t('Capture settings') }),
      jsx(CardDescription, { children: t('Only matching requests are materialized and stored after removing media, tools, and credentials.') }),
    ] }),
    jsx(CardContent, { children: jsxs('div', { className: 'conversation-archive-native', children: [
      jsxs('div', { className: 'archive-toolbar', children: [
        jsx('label', { className: 'flex items-center gap-2 text-sm', children: [
          jsx('input', { type: 'checkbox', checked: enabled, onChange: (event) => setEnabled(event.target.checked) }),
          t('Enable conversation archive'),
        ] }),
        jsx(Button, { onClick: save, disabled: saving || !config, children: saving ? t('Saving...') : t('Save settings') }),
      ] }),
      jsxs('div', { className: 'archive-grid', children: [
        jsx(GroupPicker, { groups, values: groupCodes, onChange: setGroupCodes, label: t('Groups to capture (multi-select)') }),
        jsx('div', { className: 'archive-field', children: [
          jsx('label', { children: t('User IDs (comma or space separated)') }),
          jsx('input', { value: userIds, onChange: (event) => setUserIds(event.target.value), placeholder: '1001, 1002' }),
        ] }),
        jsx('div', { className: 'archive-field', children: [
          jsx('label', { children: t('Retention days') }),
          jsx('input', { type: 'number', min: '1', max: '3650', value: retentionDays, onChange: (event) => setRetentionDays(event.target.value) }),
        ] }),
        jsx('div', { className: 'archive-field', children: [
          jsx('label', { children: t('Maximum normalized body bytes') }),
          jsx('input', { type: 'number', min: '65536', max: '2097152', value: maxBodyBytes, onChange: (event) => setMaxBodyBytes(event.target.value) }),
        ] }),
        jsx('div', { className: 'archive-field', children: [
          jsx('label', { children: t('Maximum saved conversations') }),
          jsx('input', { type: 'number', min: '1', max: '100000', value: maxArchiveCount, onChange: (event) => setMaxArchiveCount(event.target.value) }),
        ] }),
      ] }),
      jsx('p', { className: 'archive-muted', children: t('When both groups and users are set, a request must match both filters. Empty filters match all.') }),
      jsx('p', { className: 'archive-muted', children: t('Only the newest conversations are kept when this limit is reached.') }),
    ] }) }),
  ] });
}

function ArchiveList({ groups, refresh, onSelect, onCleared }) {
  const { t } = useTranslation();
  const [groupCode, setGroupCode] = useState('');
  const [userId, setUserId] = useState('');
  const [page, setPage] = useState(1);
  const [clearing, setClearing] = useState(false);
  const params = useMemo(() => ({
    page,
    page_size: 30,
    group_code: groupCode,
    user_id: userId.trim(),
  }), [groupCode, userId, page]);
  const query = useQuery({ queryKey: ['conversation-archives', params, refresh], queryFn: () => loadArchives(params) });
  const items = query.data?.items || [];
  const total = Number(query.data?.total || 0);
  const pages = Math.max(1, Math.ceil(total / 30));

  useEffect(() => setPage(1), [groupCode, userId]);

  async function clear() {
    if (typeof globalThis.confirm !== 'function' || !globalThis.confirm(t('Clear all archived conversations? This cannot be undone.'))) return;
    setClearing(true);
    try {
      await clearArchives();
      Toast?.toast?.success?.(t('Archived conversations cleared'));
      onCleared();
    } catch (error) {
      Toast?.toast?.error?.(error instanceof Error ? error.message : t('Request failed'));
    } finally {
      setClearing(false);
    }
  }

  return jsxs(Card, { children: [
    jsxs(CardHeader, { children: [
      jsx(CardTitle, { children: t('Saved conversations') }),
      jsx(CardDescription, { children: t('Click a row to load the cleaned messages for online preview.') }),
        jsxs('div', { className: 'archive-toolbar', children: [
          jsxs('div', { className: 'archive-field', children: [jsx('label', { children: t('Filter group') }), jsx('select', { value: groupCode, onChange: (event) => setGroupCode(event.target.value), children: [jsx('option', { value: '', children: t('All groups') }), (groups || []).map((group) => jsx('option', { value: group.code, children: group.name || group.code }, group.id || group.code))] })] }),
          jsxs('div', { className: 'archive-field', children: [jsx('label', { children: t('Filter user ID') }), jsx('input', { value: userId, onChange: (event) => setUserId(event.target.value), inputMode: 'numeric' })] }),
          jsx(Button, { variant: 'outline', onClick: refresh, children: t('Refresh') }),
          jsx(Button, { variant: 'destructive', disabled: clearing, onClick: clear, children: clearing ? t('Clearing...') : t('Clear archived conversations') }),
      ] }),
    ] }),
    jsx(CardContent, { className: 'p-0', children: query.isLoading ? jsx('div', { className: 'archive-muted p-6', children: t('Loading...') }) : query.error ? jsx(Alert, { variant: 'destructive', children: [jsx(AlertTitle, { children: t('Failed to load archives') }), jsx(AlertDescription, { children: query.error.message })] }) : jsxs(Fragment, { children: [
      jsx('div', { className: 'archive-table-wrap', children: jsxs(Table, { children: [
        jsx(TableHeader, { children: jsxs(TableRow, { children: [jsx(TableHead, { children: t('Created') }), jsx(TableHead, { children: t('User') }), jsx(TableHead, { children: t('Group') }), jsx(TableHead, { children: t('Model') }), jsx(TableHead, { children: t('Protocol') }), jsx(TableHead, { children: t('Messages') }), jsx(TableHead, { children: t('Size') })] }) }),
        jsx(TableBody, { children: items.length ? items.map((item) => jsx(TableRow, { onClick: () => onSelect(item.id), tabIndex: 0, onKeyDown: (event) => { if (event.key === 'Enter') onSelect(item.id); }, children: [jsx(TableCell, { children: formatTime(item.created_at) }), jsx(TableCell, { children: `${item.username || '-'} (#${item.user_id || '-'})` }), jsx(TableCell, { children: item.group_name || item.group_code || '-' }), jsx(TableCell, { children: item.model || '-' }), jsx(TableCell, { children: jsx(Badge, { variant: 'outline', children: item.protocol || '-' }) }), jsx(TableCell, { children: item.message_count }), jsx(TableCell, { children: formatBytes(item.byte_size) })] }, item.id)) : jsx(TableRow, { children: jsx(TableCell, { colSpan: 7, className: 'archive-muted text-center', children: t('No archived conversations') }) }) }),
      ] }) }),
      jsxs('div', { className: 'archive-toolbar justify-between p-3', children: [jsx('span', { className: 'archive-muted', children: t('Total {{count}}', { count: total }) }), jsxs('div', { className: 'flex gap-2', children: [jsx(Button, { variant: 'outline', size: 'sm', disabled: page <= 1, onClick: () => setPage((value) => value - 1), children: t('Previous') }), jsx(Button, { variant: 'outline', size: 'sm', disabled: page >= pages, onClick: () => setPage((value) => value + 1), children: t('Next') })] })] }),
    ] }) }),
  ] });
}

function ArchivePreview({ id, onClose }) {
  const { t } = useTranslation();
  const query = useQuery({ queryKey: ['conversation-archive', id], queryFn: () => loadArchive(id), enabled: Boolean(id) });
  const row = query.data;
  let normalized = null;
  if (row?.content) {
    try { normalized = JSON.parse(String(row.content)); } catch { normalized = null; }
  }
  return jsx(Card, { children: [
    jsxs(CardHeader, { children: [jsxs('div', { className: 'flex items-start justify-between gap-3', children: [jsx('div', { children: [jsx(CardTitle, { children: t('Conversation preview') }), row && jsx(CardDescription, { children: `${row.group_name || row.group_code || '-'} · ${row.username || `#${row.user_id}`} · ${formatTime(row.created_at)}` })] }), jsx(Button, { variant: 'outline', onClick: onClose, children: t('Close preview') })] })] }),
    jsx(CardContent, { children: query.isLoading ? jsx('div', { className: 'archive-muted', children: t('Loading...') }) : query.error ? jsx(Alert, { variant: 'destructive', children: [jsx(AlertTitle, { children: t('Failed to load conversation') }), jsx(AlertDescription, { children: query.error.message })] }) : jsx('div', { className: 'archive-preview', children: normalized?.messages?.length ? normalized.messages.map((message, index) => jsx('div', { className: 'archive-message', children: [jsx('strong', { children: message.role || t('Unknown role') }), jsx('span', { children: message.text || '' })] }, index)) : jsx('div', { className: 'archive-muted', children: t('No cleaned messages available') }) }) }),
  ] });
}

function ConversationArchivePage() {
  const { t } = useTranslation();
  const [refresh, setRefresh] = useState(0);
  const [selectedId, setSelectedId] = useState(null);
  const config = useQuery({ queryKey: ['conversation-archive-config', refresh], queryFn: loadConfig });
  const groups = useQuery({ queryKey: ['conversation-archive-groups', refresh], queryFn: loadGroups, staleTime: 30000 });
  const groupItems = Array.isArray(groups.data) ? groups.data : [];
  const bump = () => setRefresh((value) => value + 1);
  const handleCleared = () => {
    setSelectedId(null);
    bump();
  };
  return jsx(SectionPageLayout, { children: [
    jsx(SectionPageLayout.Title, { children: t('Conversation archive') }),
    jsx(SectionPageLayout.Actions, { children: config.data && jsx(Badge, { variant: config.data.enabled ? 'default' : 'secondary', children: config.data.enabled ? t('Enabled') : t('Disabled') }) }),
    jsx(SectionPageLayout.Content, { children: jsxs('div', { className: 'conversation-archive-native', children: [
      config.error && jsx(Alert, { variant: 'destructive', children: [jsx(AlertTitle, { children: t('Failed to load settings') }), jsx(AlertDescription, { children: config.error.message })] }),
      jsx(ConfigCard, { config: config.data, groups: groupItems, refresh: bump }),
      selectedId ? jsx(ArchivePreview, { id: selectedId, onClose: () => setSelectedId(null) }) : null,
      jsx(ArchiveList, { groups: groupItems, refresh, onSelect: setSelectedId, onCleared: handleCleared }),
    ] }) }),
  ] });
}

export default ConversationArchivePage;
