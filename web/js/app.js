// === API client ===
const API = {
  async get(path) { const r = await fetch(path); return r.json(); },
  async put(path, body) { const r = await fetch(path, { method: 'PUT', headers: {'Content-Type':'application/json'}, body: JSON.stringify(body) }); return r.json(); },
  async postForm(path, yaml) { const r = await fetch(path, { method: 'POST', headers: {'Content-Type':'application/x-yaml'}, body: yaml }); return r.json(); },
  async exportConfig() { const r = await fetch('/api/config/export'); return r.text(); }
};

// === Router ===
const routes = {};
const $content = document.getElementById('content');

function register(name, fn) { routes[name] = fn; }
function navigate(hash) {
  const page = (hash || location.hash || '#dashboard').slice(1);
  document.querySelectorAll('.nav-link').forEach(l => l.classList.toggle('active', l.dataset.page === page));
  const fn = routes[page];
  if (fn) fn();
}

window.addEventListener('hashchange', () => navigate());
document.addEventListener('DOMContentLoaded', () => navigate());

// === Toast ===
let toastTimer;
function toast(msg, isError) {
  const t = document.getElementById('toast') || createToast();
  t.textContent = msg;
  t.className = 'show' + (isError ? ' error' : '');
  clearTimeout(toastTimer);
  toastTimer = setTimeout(() => t.className = '', 3000);
}
function createToast() {
  const t = document.createElement('div'); t.id = 'toast';
  document.body.appendChild(t); return t;
}

// === Topbar refresh ===
async function refreshTopbar() {
  try {
    const s = await API.get('/api/status');
    const badge = document.getElementById('statusBadge');
    const uptime = document.getElementById('uptime');
    const reqCount = document.getElementById('reqCount');
    if (s.running) {
      badge.textContent = '● Running';
      badge.className = 'status-badge running';
    } else {
      badge.textContent = '● Stopped';
      badge.className = 'status-badge stopped';
    }
    uptime.textContent = 'Uptime: ' + fmtDuration(s.uptime_seconds);
    reqCount.textContent = 'Requests: ' + s.request_count;
  } catch(e) {}
}
function fmtDuration(s) {
  if (s < 60) return s + 's';
  if (s < 3600) return Math.floor(s/60) + 'm ' + (s%60) + 's';
  return Math.floor(s/3600) + 'h ' + Math.floor((s%3600)/60) + 'm';
}
setInterval(refreshTopbar, 5000);
refreshTopbar();

// === Dashboard ===
register('dashboard', async () => {
  $content.innerHTML = '<h2>Dashboard</h2><div id="dashContent">Loading...</div>';
  const s = await API.get('/api/status');
  const $d = document.getElementById('dashContent');
  $d.innerHTML = `
    <div class="card-row">
      <div class="card card-stat"><div class="label">Status</div><div class="value" style="color:${s.running?'var(--green)':'var(--red)'}">${s.running ? 'Running' : 'Stopped'}</div></div>
      <div class="card card-stat"><div class="label">Uptime</div><div class="value">${fmtDuration(s.uptime_seconds)}</div></div>
      <div class="card card-stat"><div class="label">Requests</div><div class="value">${s.request_count}</div></div>
    </div>
    <div class="card">
      <div style="display:flex;justify-content:space-between;align-items:center;">
        <div>
          <p style="color:var(--text2);font-size:13px;">Proxy: <strong>${s.proxy_addr}</strong></p>
          <p style="color:var(--text2);font-size:13px;">Admin: <strong>${s.admin_addr}</strong></p>
          <p style="color:var(--text2);font-size:13px;">Config: <strong>${s.config_path}</strong></p>
          ${s.has_default_upstream ? `<p style="color:var(--text2);font-size:13px;">Default Upstream: <strong>${escapeHtml(s.default_upstream_url)}</strong></p>` : ''}
          <p style="color:var(--text2);font-size:13px;">Active Models: ${s.active_models.length ? s.active_models.join(', ') : '(none)'}</p>
        </div>
      </div>
    </div>`;
});

// === Models ===
register('models', async () => {
  renderModels();
});

async function renderModels() {
  const cfg = await API.get('/api/config');
  const models = cfg.models || [];
  $content.innerHTML = `<h2>Models</h2>
    <button class="btn btn-primary" onclick="showModelModal()">+ Add Model</button>
    <br><br>
    <table>
      <thead><tr><th>Model Name</th><th>Upstream URL</th><th>Rewrite To</th><th>API Key</th><th></th></tr></thead>
      <tbody>${models.map((m,i) => `<tr>
        <td><strong>${escapeHtml(m.name)}</strong></td>
        <td class="mono">${escapeHtml(m.upstream.url)}</td>
        <td>${m.upstream.model_name || '<em style=color:var(--text2)>passthrough</em>'}</td>
        <td class="mono">${escapeHtml(m.upstream.api_key)}</td>
        <td class="actions">
          <button class="btn btn-outline btn-sm" onclick="showModelModal(${i})">Edit</button>
          <button class="btn btn-outline btn-sm" style="color:var(--red);border-color:var(--red)" onclick="deleteModel(${i})">Delete</button>
        </td>
      </tr>`).join('')}</tbody>
    </table>
    <div class="modal-overlay" id="modelModal">
      <div class="modal">
        <h3 id="modelModalTitle">Add Model</h3>
        <div class="form-group"><label>Name</label><input id="mName" placeholder="claude-sonnet-4-20250514"></div>
        <div class="form-group"><label>Upstream URL</label><input id="mUrl" placeholder="https://api.deepseek.com/anthropic/v1/messages"></div>
        <div class="form-group"><label>API Key</label><input id="mKey" placeholder="sk-..."></div>
        <div class="form-group"><label>Model Name Override</label><input id="mOverride" placeholder="deepseek-v4-pro (leave empty to passthrough)"></div>
        <p id="mError" style="color:var(--red);font-size:12px;display:none"></p>
        <div class="modal-actions">
          <button class="btn btn-outline" onclick="closeModelModal()">Cancel</button>
          <button class="btn btn-primary" onclick="saveModel()">Save</button>
        </div>
      </div>
    </div>`;
  // Store models for the modal
  window._models = models;
  window._cfg = cfg;
}

window.showModelModal = function(idx) {
  const modal = document.getElementById('modelModal');
  const title = document.getElementById('modelModalTitle');
  modal.classList.add('show');
  window._editIdx = idx !== undefined ? idx : -1;
  if (idx !== undefined && idx >= 0) {
    title.textContent = 'Edit Model';
    const m = window._models[idx];
    document.getElementById('mName').value = m.name;
    document.getElementById('mUrl').value = m.upstream.url;
    document.getElementById('mKey').value = m.upstream.api_key;
    document.getElementById('mOverride').value = m.upstream.model_name || '';
  } else {
    title.textContent = 'Add Model';
    document.getElementById('mName').value = '';
    document.getElementById('mUrl').value = '';
    document.getElementById('mKey').value = '';
    document.getElementById('mOverride').value = '';
  }
  document.getElementById('mError').style.display = 'none';
};

window.closeModelModal = function() {
  document.getElementById('modelModal').classList.remove('show');
};

window.saveModel = async function() {
  const name = document.getElementById('mName').value.trim();
  const url = document.getElementById('mUrl').value.trim();
  const key = document.getElementById('mKey').value.trim();
  const override = document.getElementById('mOverride').value.trim();
  if (!name || !url || !key) {
    const e = document.getElementById('mError');
    e.textContent = 'Name, URL and API Key are required';
    e.style.display = 'block';
    return;
  }
  const model = { name, upstream: { url, api_key: key, model_name: override } };
  const models = [...window._models];
  if (window._editIdx >= 0) {
    models[window._editIdx] = model;
  } else {
    models.push(model);
  }
  const newCfg = { ...window._cfg, models };
  const result = await API.put('/api/config', newCfg);
  if (result.error) {
    document.getElementById('mError').textContent = result.error;
    document.getElementById('mError').style.display = 'block';
    return;
  }
  closeModelModal();
  toast('Model saved');
  renderModels();
};

window.deleteModel = async function(idx) {
  if (!confirm('Delete model "' + window._models[idx].name + '"?')) return;
  const models = [...window._models];
  models.splice(idx, 1);
  await API.put('/api/config', { ...window._cfg, models });
  toast('Model deleted');
  renderModels();
};

// === Default Upstream ===
register('default', async () => {
  const cfg = await API.get('/api/config');
  const d = cfg.default || { url: '', api_key: '', model_name: '' };
  $content.innerHTML = `<h2>Default Upstream</h2>
    <p style="color:var(--text2);margin-bottom:16px;">Catch-all for any model not in the explicit list.</p>
    <div class="card">
      <div class="form-group"><label>URL</label><input id="dUrl" value="${escapeAttr(d.url)}" placeholder="https://api.deepseek.com/anthropic/v1/messages"></div>
      <div class="form-group"><label>API Key</label><input id="dKey" value="${escapeAttr(d.api_key)}" placeholder="sk-..."></div>
      <div class="form-group"><label>Model Name Override</label><input id="dOverride" value="${escapeAttr(d.model_name)}" placeholder="deepseek-v4-pro (leave empty to passthrough)"></div>
      <button class="btn btn-primary" onclick="saveDefault()">Save</button>
      <button class="btn btn-danger" style="margin-left:8px" onclick="removeDefault()">Disable Default</button>
      <p id="dError" style="color:var(--red);font-size:12px;margin-top:8px;display:none"></p>
    </div>`;
  window._cfg = cfg;
});

window.saveDefault = async function() {
  const url = document.getElementById('dUrl').value.trim();
  const key = document.getElementById('dKey').value.trim();
  const override = document.getElementById('dOverride').value.trim();
  if (!url || !key) {
    const e = document.getElementById('dError'); e.textContent = 'URL and API Key are required'; e.style.display = 'block'; return;
  }
  const result = await API.put('/api/config', { ...window._cfg, default: { url, api_key: key, model_name: override }});
  if (result.error) {
    document.getElementById('dError').textContent = result.error; document.getElementById('dError').style.display = 'block';
    return;
  }
  toast('Default upstream saved');
};

window.removeDefault = async function() {
  if (!confirm('Disable default upstream?')) return;
  await API.put('/api/config', { ...window._cfg, default: null, models: window._cfg.models });
  toast('Default upstream disabled');
  window._cfg.default = null;
  document.getElementById('dUrl').value = '';
  document.getElementById('dKey').value = '';
  document.getElementById('dOverride').value = '';
};

// === Settings ===
register('settings', async () => {
  const cfg = await API.get('/api/config');
  window._cfg = cfg;
  const timeout = cfg.upstream_timeout ? cfg.upstream_timeout / 1e9 : 120;
  const connTimeout = cfg.upstream_connect_timeout ? cfg.upstream_connect_timeout / 1e9 : 10;
  $content.innerHTML = `<h2>Settings</h2>
    <div class="card">
      <div class="form-row">
        <div class="form-group"><label>Proxy Listen</label><input id="sListen" value="${escapeAttr(cfg.listen)}"></div>
        <div class="form-group"><label>Admin Listen</label><input id="sAdminListen" value="${escapeAttr(cfg.admin_listen)}"></div>
      </div>
      <div class="form-row">
        <div class="form-group"><label>Upstream Timeout (sec)</label><input id="sTimeout" type="number" value="${timeout}"></div>
        <div class="form-group"><label>Connect Timeout (sec)</label><input id="sConnTimeout" type="number" value="${connTimeout}"></div>
      </div>
      <div class="form-group">
        <label class="toggle-label"><input type="checkbox" id="sCors" ${cfg.cors.enabled?'checked':''}> CORS Enabled</label>
      </div>
      <div class="form-group"><label>Allowed Origins (one per line)</label><textarea id="sOrigins">${(cfg.cors.allowed_origins||['*']).join('\n')}</textarea></div>
      <button class="btn btn-primary" onclick="saveSettings()">Save</button>
      <p id="sError" style="color:var(--red);font-size:12px;margin-top:8px;display:none"></p>
    </div>`;
});

window.saveSettings = async function() {
  const listen = document.getElementById('sListen').value.trim();
  const adminListen = document.getElementById('sAdminListen').value.trim();
  const timeout = parseInt(document.getElementById('sTimeout').value) || 120;
  const connTimeout = parseInt(document.getElementById('sConnTimeout').value) || 10;
  const corsEnabled = document.getElementById('sCors').checked;
  const origins = document.getElementById('sOrigins').value.split('\n').map(s=>s.trim()).filter(Boolean);
  const update = {
    ...window._cfg,
    listen,
    admin_listen: adminListen,
    upstream_timeout: timeout * 1e9,
    upstream_connect_timeout: connTimeout * 1e9,
    cors: { enabled: corsEnabled, allowed_origins: origins }
  };
  const result = await API.put('/api/config', update);
  if (result.error) {
    document.getElementById('sError').textContent = result.error; document.getElementById('sError').style.display = 'block';
    return;
  }
  window._cfg = result;
  toast('Settings saved. Restart to apply listen address changes.');
};

// === Logs ===
let logEventSource = null;
let logEntryCount = 0;

register('logs', () => {
  $content.innerHTML = `<h2>Logs</h2>
    <div class="filter-bar">
      <select id="logLevel"><option value="">All levels</option><option>info</option><option>warn</option><option>error</option></select>
      <select id="logMethod"><option value="">All methods</option><option>POST</option><option>GET</option></select>
      <input id="logModel" placeholder="Filter model...">
      <label class="toggle-label"><input type="checkbox" id="autoScroll" checked> Auto-scroll</label>
      <span id="logConnStatus" class="log-status connecting"><span class="log-status-dot"></span> Connecting</span>
      <button class="btn btn-outline btn-sm" onclick="clearLogs()">Clear</button>
    </div>
    <div id="logList">
      <div class="log-empty"><div class="log-empty-icon">-</div>Waiting for requests...</div>
    </div>`;
  startLogStream();
});

function clearLogs() {
  document.getElementById('logList').innerHTML = '<div class="log-empty"><div class="log-empty-icon">-</div>Waiting for requests...</div>';
  logEntryCount = 0;
}

function setLogStatus(state) {
  const el = document.getElementById('logConnStatus');
  if (!el) return;
  el.className = 'log-status ' + state;
  el.innerHTML = '<span class="log-status-dot"></span> ' + (state === 'connecting' ? 'Connecting' : state === 'connected' ? 'Live' : 'Disconnected');
}

function startLogStream() {
  if (logEventSource) logEventSource.close();
  logEntryCount = 0;
  setLogStatus('connecting');

  logEventSource = new EventSource('/api/logs');
  logEventSource.addEventListener('log', e => {
    const entry = JSON.parse(e.data);
    appendLogEntry(entry);
  });
  logEventSource.onopen = () => setLogStatus('connected');
  logEventSource.onerror = () => {
    setLogStatus('disconnected');
    // EventSource will auto-reconnect
  };
}

function appendLogEntry(entry) {
  const level = document.getElementById('logLevel')?.value || '';
  const method = document.getElementById('logMethod')?.value || '';
  const model = document.getElementById('logModel')?.value || '';
  if (level && entry.level !== level) return;
  if (method && entry.method !== method) return;
  if (model && !(entry.model||'').includes(model)) return;

  const $list = document.getElementById('logList');
  if (!$list) return;

  // Remove empty state on first entry
  if (logEntryCount === 0) {
    $list.innerHTML = '';
  }
  logEntryCount++;

  const $div = document.createElement('div');
  $div.className = 'log-entry';
  $div.innerHTML = `
    <span class="time">${new Date(entry.timestamp).toLocaleTimeString()}</span>
    <span class="level ${entry.level}">${entry.level.toUpperCase()}</span>
    <span class="method-badge">${entry.method}</span>
    <span class="path">${escapeHtml(entry.path)}</span>
    <span class="status s${Math.floor(entry.status/100)*100}">${entry.status}</span>
    <span class="duration">${entry.duration}</span>
    ${entry.model ? `<span class="model">${escapeHtml(entry.model)}</span>` : ''}`;
  $div.addEventListener('click', () => {
    const detail = $div.querySelector('.log-detail');
    if (detail) { detail.remove(); return; }
    const d = document.createElement('div'); d.className = 'log-detail';
    d.style.cssText = 'font-size:11px;color:var(--text2);margin-top:4px';
    d.textContent = `Remote: ${entry.remote}`;
    $div.appendChild(d);
  });
  $list.appendChild($div);

  const autoScroll = document.getElementById('autoScroll');
  if (autoScroll?.checked) {
    $list.scrollTop = $list.scrollHeight;
  }

  // Keep only last 500 entries in DOM
  while ($list.children.length > 500) $list.firstChild.remove();
}

// === Import ===
register('import', () => {
  $content.innerHTML = `<h2>Import Config</h2>
    <div class="card">
      <p style="color:var(--text2);margin-bottom:12px;">Select a YAML config file to import.</p>
      <input type="file" id="importFile" accept=".yaml,.yml" onchange="importConfig()">
      <p id="importResult" style="margin-top:8px;font-size:13px"></p>
    </div>`;
});

window.importConfig = async function() {
  const file = document.getElementById('importFile').files[0];
  if (!file) return;
  const text = await file.text();
  const result = await API.postForm('/api/config/import', text);
  const p = document.getElementById('importResult');
  if (result.error) {
    p.style.color = 'var(--red)'; p.textContent = 'Error: ' + result.error;
  } else {
    p.style.color = 'var(--green)'; p.textContent = 'Config imported successfully.';
    toast('Config imported');
  }
};

// === Export ===
register('export', async () => {
  $content.innerHTML = `<h2>Export Config</h2>
    <div class="card">
      <p style="color:var(--text2);margin-bottom:12px;">Download the current configuration as YAML.</p>
      <button class="btn btn-primary" onclick="doExport()">Download config.yaml</button>
    </div>`;
});

window.doExport = async function() {
  const yaml = await API.exportConfig();
  const blob = new Blob([yaml], { type: 'application/x-yaml' });
  const url = URL.createObjectURL(blob);
  const a = document.createElement('a');
  a.href = url; a.download = 'config.yaml';
  a.click();
  URL.revokeObjectURL(url);
  toast('Config exported');
};

// === Helpers ===
function escapeHtml(s) {
  if (!s) return '';
  return s.replace(/&/g,'&amp;').replace(/</g,'&lt;').replace(/>/g,'&gt;').replace(/"/g,'&quot;');
}
function escapeAttr(s) {
  if (!s) return '';
  return s.replace(/&/g,'&amp;').replace(/"/g,'&quot;').replace(/</g,'&lt;').replace(/>/g,'&gt;');
}
