const sdk = globalThis.__NEW_API_EXTENSION_NATIVE_SDK__;
if (!sdk || sdk.sdk !== 'v1' || sdk.platform !== 'classic') {
  throw new Error('Native extension SDK v1 for classic is unavailable.');
}

const React = sdk.modules.react;
const Runtime = sdk.modules['react/jsx-runtime'];
const I18n = sdk.modules['react-i18next'];
const Helper = sdk.modules['../../helpers'];
const required = [
  ['react', React],
  ['react/jsx-runtime', Runtime],
  ['react-i18next', I18n],
  ['../../helpers', Helper],
];
const missing = required.filter(([, module]) => !module).map(([name]) => name);
if (missing.length) {
  throw new Error(`Classic host SDK modules are unavailable: ${missing.join(', ')}`);
}

const { jsx, jsxs, Fragment } = Runtime;
const { useEffect, useState } = React;
const { useTranslation } = I18n;
const getAPI = typeof Helper.getAPI === 'function'
  ? Helper.getAPI
  : () => Helper.API;
const basePath = '/api/extensions/conversation-archive';

function unwrap(response) {
  const payload = response?.data ?? response;
  if (payload?.success === false) {
    throw new Error(payload.message || 'Request failed');
  }
  return payload?.data ?? payload;
}

function apiOptions() {
  return { skipErrorHandler: true };
}

async function loadConfig() {
  return unwrap(await getAPI().get(`${basePath}/config`, apiOptions()));
}

async function loadGroups() {
  return unwrap(await getAPI().get(`${basePath}/groups`, apiOptions()));
}

async function loadArchives(params) {
  return unwrap(await getAPI().get(`${basePath}/conversations`, { ...apiOptions(), params }));
}

async function loadArchive(id) {
  return unwrap(await getAPI().get(`${basePath}/conversations/${id}`, apiOptions()));
}

async function clearArchives() {
  return unwrap(await getAPI().post(`${basePath}/conversations/clear`, { confirm: true }, apiOptions()));
}

export async function loadConversationArchiveData() {
  const config = await loadConfig();
  return {
    config,
    groups: loadGroups()
      .then((value) => ({
        value: Array.isArray(value) ? value : [],
        error: null,
      }))
      .catch((error) => ({
        value: [],
        error: error instanceof Error ? error : new Error('Request failed'),
      })),
  };
}

function formatTime(value) {
  if (!value) return '-';
  const date = new Date(Number(value) * 1000);
  return Number.isNaN(date.getTime()) ? '-' : new Intl.DateTimeFormat(undefined, {
    year: 'numeric', month: '2-digit', day: '2-digit',
    hour: '2-digit', minute: '2-digit', second: '2-digit',
  }).format(date);
}

function formatBytes(value) {
  const size = Number(value || 0);
  if (size < 1024) return `${size} B`;
  if (size < 1024 * 1024) return `${(size / 1024).toFixed(1)} KiB`;
  return `${(size / 1024 / 1024).toFixed(2)} MiB`;
}

function parseUserIds(value) {
  return [...new Set(String(value || '').split(/[\s,，]+/)
    .map((item) => Number(item))
    .filter((id) => Number.isInteger(id) && id > 0))];
}

function ErrorMessage({ message }) {
  return jsx('div', { className: 'archive-error', role: 'alert', children: message });
}

function ConfigCard({ config, groups, onRefresh }) {
  const { t } = useTranslation();
  const [enabled, setEnabled] = useState(Boolean(config?.enabled));
  const [groupCodes, setGroupCodes] = useState(config?.group_codes || []);
  const [userIds, setUserIds] = useState((config?.user_ids || []).join(', '));
  const [retentionDays, setRetentionDays] = useState(String(config?.retention_days || 30));
  const [maxBodyBytes, setMaxBodyBytes] = useState(String(config?.max_body_bytes || 2097152));
  const [maxArchiveCount, setMaxArchiveCount] = useState(String(config?.max_archive_count || 1000));
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState(null);

  useEffect(() => {
    setEnabled(Boolean(config?.enabled));
    setGroupCodes(config?.group_codes || []);
    setUserIds((config?.user_ids || []).join(', '));
    setRetentionDays(String(config?.retention_days || 30));
    setMaxBodyBytes(String(config?.max_body_bytes || 2097152));
    setMaxArchiveCount(String(config?.max_archive_count || 1000));
  }, [config?.config_version]);

  async function save() {
    if (!config) return;
    setSaving(true);
    setError(null);
    try {
      unwrap(await getAPI().put(`${basePath}/config`, {
        expected_version: Number(config.config_version || 0),
        enabled,
        group_codes: groupCodes,
        user_ids: parseUserIds(userIds),
        retention_days: Number(retentionDays),
        max_body_bytes: Number(maxBodyBytes),
        max_archive_count: Number(maxArchiveCount),
      }, apiOptions()));
      onRefresh();
    } catch (saveError) {
      setError(saveError instanceof Error ? saveError.message : t('Request failed'));
    } finally {
      setSaving(false);
    }
  }

  return jsxs('section', { className: 'archive-card', children: [
    jsxs('div', { className: 'archive-card-header', children: [
      jsx('h2', { children: t('Capture settings') }),
      jsx('p', { children: t('Only matching requests are materialized and stored after removing media, tools, and credentials.') }),
    ] }),
    jsxs('div', { className: 'archive-card-content', children: [
      error ? jsx(ErrorMessage, { message: error }) : null,
      jsxs('div', { className: 'archive-toolbar', children: [
        jsx('label', { className: 'archive-checkbox', children: [
          jsx('input', { type: 'checkbox', checked: enabled, onChange: (event) => setEnabled(event.target.checked) }),
          t('Enable conversation archive'),
        ] }),
        jsx('button', { type: 'button', disabled: saving || !config, onClick: save, children: saving ? t('Saving...') : t('Save settings') }),
      ] }),
      jsxs('div', { className: 'archive-grid', children: [
        jsxs('label', { className: 'archive-field', children: [
          t('Groups to capture (multi-select)'),
          jsx('select', { multiple: true, value: groupCodes, onChange: (event) => setGroupCodes([...event.target.selectedOptions].map((option) => option.value)), children: (groups || []).map((group) => jsx('option', { value: group.code, children: `${group.name || group.code} (${group.code})` }, group.id || group.code)) }),
        ] }),
        jsxs('label', { className: 'archive-field', children: [
          t('User IDs (comma or space separated)'),
          jsx('input', { value: userIds, onChange: (event) => setUserIds(event.target.value), placeholder: '1001, 1002' }),
        ] }),
        jsxs('label', { className: 'archive-field', children: [
          t('Retention days'),
          jsx('input', { type: 'number', min: '1', max: '3650', value: retentionDays, onChange: (event) => setRetentionDays(event.target.value) }),
        ] }),
        jsxs('label', { className: 'archive-field', children: [
          t('Maximum normalized body bytes'),
          jsx('input', { type: 'number', min: '65536', max: '2097152', value: maxBodyBytes, onChange: (event) => setMaxBodyBytes(event.target.value) }),
        ] }),
        jsxs('label', { className: 'archive-field', children: [
          t('Maximum saved conversations'),
          jsx('input', { type: 'number', min: '1', max: '100000', value: maxArchiveCount, onChange: (event) => setMaxArchiveCount(event.target.value) }),
        ] }),
      ] }),
      jsx('p', { className: 'archive-muted', children: t('When both groups and users are set, a request must match both filters. Empty filters match all.') }),
      jsx('p', { className: 'archive-muted', children: t('Only the newest conversations are kept when this limit is reached.') }),
    ] }),
  ] });
}

function ArchiveList({ groups, refreshKey, onSelect, onCleared }) {
  const { t } = useTranslation();
  const [groupCode, setGroupCode] = useState('');
  const [userId, setUserId] = useState('');
  const [page, setPage] = useState(1);
  const [clearing, setClearing] = useState(false);
  const [state, setState] = useState({ loading: true, error: null, data: null });
  const total = Number(state.data?.total || 0);
  const pages = Math.max(1, Math.ceil(total / 30));

  useEffect(() => {
    setPage(1);
  }, [groupCode, userId]);

  useEffect(() => {
    let active = true;
    setState({ loading: true, error: null, data: null });
    loadArchives({ page, page_size: 30, group_code: groupCode, user_id: userId.trim() })
      .then((data) => active && setState({ loading: false, error: null, data }))
      .catch((error) => active && setState({ loading: false, error: error instanceof Error ? error.message : t('Request failed'), data: null }));
    return () => { active = false; };
  }, [groupCode, page, refreshKey, t, userId]);

  async function clear() {
    if (typeof globalThis.confirm !== 'function' || !globalThis.confirm(t('Clear all archived conversations? This cannot be undone.'))) return;
    setClearing(true);
    try {
      await clearArchives();
      onCleared();
    } catch (error) {
      setState((current) => ({ ...current, error: error instanceof Error ? error.message : t('Request failed') }));
    } finally {
      setClearing(false);
    }
  }

  const items = state.data?.items || [];
  return jsxs('section', { className: 'archive-card', children: [
    jsxs('div', { className: 'archive-card-header', children: [
      jsx('h2', { children: t('Saved conversations') }),
      jsx('p', { children: t('Click a row to load the cleaned messages for online preview.') }),
        jsxs('div', { className: 'archive-toolbar', children: [
          jsxs('label', { className: 'archive-field', children: [t('Filter group'), jsx('select', { value: groupCode, onChange: (event) => setGroupCode(event.target.value), children: [jsx('option', { value: '', children: t('All groups') }), (groups || []).map((group) => jsx('option', { value: group.code, children: group.name || group.code }, group.id || group.code))] })] }),
          jsxs('label', { className: 'archive-field', children: [t('Filter user ID'), jsx('input', { value: userId, onChange: (event) => setUserId(event.target.value), inputMode: 'numeric' })] }),
          jsx('button', { className: 'archive-danger-button', type: 'button', disabled: clearing, onClick: clear, children: clearing ? t('Clearing...') : t('Clear archived conversations') }),
      ] }),
    ] }),
    jsx('div', { className: 'archive-card-content archive-card-content-flush', children: state.loading ? jsx('div', { className: 'archive-muted archive-content-padding', children: t('Loading...') }) : state.error ? jsx(ErrorMessage, { message: state.error }) : jsxs(Fragment, { children: [
      jsx('div', { className: 'archive-table-wrap', children: jsxs('table', { children: [
        jsx('thead', { children: jsx('tr', { children: ['Created', 'User', 'Group', 'Model', 'Protocol', 'Messages', 'Size'].map((title) => jsx('th', { children: t(title) }, title)) }) }),
        jsx('tbody', { children: items.length ? items.map((item) => jsx('tr', { onClick: () => onSelect(item.id), tabIndex: 0, onKeyDown: (event) => { if (event.key === 'Enter') onSelect(item.id); }, children: [jsx('td', { children: formatTime(item.created_at) }), jsx('td', { children: `${item.username || '-'} (#${item.user_id || '-'})` }), jsx('td', { children: item.group_name || item.group_code || '-' }), jsx('td', { children: item.model || '-' }), jsx('td', { children: item.protocol || '-' }), jsx('td', { children: item.message_count }), jsx('td', { children: formatBytes(item.byte_size) })] }, item.id)) : jsx('tr', { children: jsx('td', { colSpan: 7, className: 'archive-muted text-center', children: t('No archived conversations') }) }) }),
      ] }) }),
      jsxs('div', { className: 'archive-toolbar archive-pagination', children: [jsx('span', { className: 'archive-muted', children: t('Total {{count}}', { count: total }) }), jsxs('div', { className: 'archive-toolbar', children: [jsx('button', { type: 'button', disabled: page <= 1, onClick: () => setPage((value) => value - 1), children: t('Previous') }), jsx('button', { type: 'button', disabled: page >= pages, onClick: () => setPage((value) => value + 1), children: t('Next') })] })] }),
    ] }) }),
  ] });
}

function ArchivePreview({ id, onClose }) {
  const { t } = useTranslation();
  const [state, setState] = useState({ loading: true, error: null, data: null });
  useEffect(() => {
    let active = true;
    setState({ loading: true, error: null, data: null });
    loadArchive(id)
      .then((data) => active && setState({ loading: false, error: null, data }))
      .catch((error) => active && setState({ loading: false, error: error instanceof Error ? error.message : t('Request failed'), data: null }));
    return () => { active = false; };
  }, [id, t]);

  let normalized = null;
  if (state.data?.content) {
    try { normalized = JSON.parse(String(state.data.content)); } catch { normalized = null; }
  }
  return jsxs('section', { className: 'archive-card', children: [
    jsxs('div', { className: 'archive-card-header archive-toolbar justify-between', children: [
      jsxs('div', { children: [jsx('h2', { children: t('Conversation preview') }), state.data ? jsx('p', { children: `${state.data.group_name || state.data.group_code || '-'} · ${state.data.username || `#${state.data.user_id}`} · ${formatTime(state.data.created_at)}` }) : null] }),
      jsx('button', { type: 'button', onClick: onClose, children: t('Close preview') }),
    ] }),
    jsx('div', { className: 'archive-card-content', children: state.loading ? jsx('div', { className: 'archive-muted', children: t('Loading...') }) : state.error ? jsx(ErrorMessage, { message: state.error }) : jsx('div', { className: 'archive-preview', children: normalized?.messages?.length ? normalized.messages.map((message, index) => jsx('div', { className: 'archive-message', children: [jsx('strong', { children: message.role || t('Unknown role') }), jsx('span', { children: message.text || '' })] }, index)) : jsx('div', { className: 'archive-muted', children: t('No cleaned messages available') }) }) }),
  ] });
}

function ConversationArchivePage() {
  const { t } = useTranslation();
  const [refreshKey, setRefreshKey] = useState(0);
  const [selectedId, setSelectedId] = useState(null);
  const [state, setState] = useState({
    loading: true,
    error: null,
    config: null,
    groups: [],
    groupsError: null,
  });

  useEffect(() => {
    let active = true;
    setState((current) => ({ ...current, loading: true, error: null }));
    loadConversationArchiveData()
      .then((data) => {
        if (active) {
          setState({
            loading: false,
            error: null,
            config: data.config,
            groups: [],
            groupsError: null,
          });
          void data.groups.then(({ value, error }) => {
            if (active) {
              setState((current) => ({
                ...current,
                groups: value,
                groupsError: error?.message || null,
              }));
            }
          });
        }
      })
      .catch((error) => active && setState((current) => ({
        ...current,
        loading: false,
        error: error instanceof Error ? error.message : t('Request failed'),
      })));
    return () => { active = false; };
  }, [refreshKey, t]);

  const refresh = () => setRefreshKey((value) => value + 1);
  const handleCleared = () => {
    setSelectedId(null);
    refresh();
  };
  return jsxs('div', { className: 'conversation-archive-native', children: [
    jsxs('div', { className: 'archive-page-header', children: [jsx('h1', { children: t('Conversation archive') }), jsx('button', { type: 'button', onClick: refresh, children: t('Refresh') })] }),
    state.error ? jsx(ErrorMessage, { message: state.error }) : null,
    state.groupsError ? jsx(ErrorMessage, { message: state.groupsError }) : null,
    state.loading && !state.config ? jsx('div', { className: 'archive-muted', children: t('Loading...') }) : null,
    state.config ? jsx(ConfigCard, { config: state.config, groups: state.groups, onRefresh: refresh }) : null,
    selectedId ? jsx(ArchivePreview, { id: selectedId, onClose: () => setSelectedId(null) }) : null,
    jsx(ArchiveList, { groups: state.groups, refreshKey, onSelect: setSelectedId, onCleared: handleCleared }),
  ] });
}

export default ConversationArchivePage;
