// Koor Dashboard — polls the API server every 5 seconds.
// The dashboard runs on a separate port, so API_BASE is configurable.
// It reads the api-base from a meta tag or defaults to the same origin.
const API_BASE = document.querySelector('meta[name="api-base"]')?.content || '';

async function fetchJSON(path) {
  try {
    const resp = await fetch(API_BASE + path);
    if (!resp.ok) return null;
    return await resp.json();
  } catch {
    return null;
  }
}

function renderTable(pairs) {
  if (!pairs || pairs.length === 0) return '<p class="empty">No data</p>';
  let html = '<table>';
  for (const [k, v] of pairs) {
    html += `<tr><td>${esc(k)}</td><td>${esc(String(v))}</td></tr>`;
  }
  html += '</table>';
  return html;
}

function esc(s) {
  const d = document.createElement('div');
  d.textContent = s;
  return d.innerHTML;
}

async function refreshHealth() {
  const data = await fetchJSON('/health');
  const el = document.getElementById('health-info');
  const status = document.getElementById('status');

  if (!data) {
    el.innerHTML = '<p class="empty">Unreachable</p>';
    status.textContent = 'error';
    status.className = 'status error';
    return;
  }

  status.textContent = data.status;
  status.className = 'status ok';
  el.innerHTML = renderTable([
    ['Status', data.status],
    ['Uptime', data.uptime],
  ]);
}

async function refreshTokenTax() {
  const data = await fetchJSON('/api/metrics');
  const el = document.getElementById('token-tax-info');

  if (!data || !data.token_tax) {
    el.innerHTML = '<p class="empty">No token tax data</p>';
    return;
  }

  const tt = data.token_tax;
  const saved = tt.rest_tokens_saved || 0;
  const mcpTokens = tt.mcp_estimated_tokens || 0;
  const pct = tt.savings_percent || 0;
  const barWidth = Math.min(pct, 100);

  el.innerHTML = `
    <div class="tt-big-number">${saved.toLocaleString()} tokens saved</div>
    <div class="tt-bar-container">
      <div class="tt-bar-fill" style="width:${barWidth}%"></div>
      <span class="tt-bar-label">${pct.toFixed(1)}% bypass rate</span>
    </div>
    <div class="tt-stats">
      <div class="tt-stat">
        <span class="tt-stat-value">${tt.rest_calls}</span>
        <span class="tt-stat-label">REST/CLI calls</span>
      </div>
      <div class="tt-stat">
        <span class="tt-stat-value">${tt.mcp_calls}</span>
        <span class="tt-stat-label">MCP calls</span>
      </div>
      <div class="tt-stat">
        <span class="tt-stat-value">${mcpTokens.toLocaleString()}</span>
        <span class="tt-stat-label">MCP tokens used</span>
      </div>
    </div>
    <p class="tt-explainer">MCP calls flow through the LLM context window (cost tokens). REST/CLI calls bypass it entirely (zero tokens).</p>
  `;
}

async function refreshInstances() {
  const data = await fetchJSON('/api/instances');
  const el = document.getElementById('instances-info');

  if (!data || data.length === 0) {
    el.innerHTML = '<p class="empty">No instances registered</p>';
    return;
  }

  let html = '<table>';
  html += '<tr><td><strong>Name</strong></td><td><strong>Status</strong></td><td><strong>Intent</strong></td></tr>';
  for (const inst of data) {
    const status = inst.status || 'pending';
    const badgeClass = status === 'active' ? 'badge-ok' : 'badge-warning';
    html += `<tr><td>${esc(inst.name)}</td><td><span class="badge ${badgeClass}">${esc(status)}</span></td><td>${esc(inst.intent || '-')}</td></tr>`;
  }
  html += '</table>';
  el.innerHTML = html;
}

async function refreshState() {
  const data = await fetchJSON('/api/state');
  const el = document.getElementById('state-info');

  if (!data || data.length === 0) {
    el.innerHTML = '<p class="empty">No state keys</p>';
    return;
  }

  let html = '<table>';
  html += '<tr><td><strong>Key</strong></td><td><strong>Version</strong></td></tr>';
  for (const item of data) {
    html += `<tr><td>${esc(item.key)}</td><td>v${item.version}</td></tr>`;
  }
  html += '</table>';
  el.innerHTML = html;
}

async function refreshEvents() {
  const data = await fetchJSON('/api/events/history?last=10');
  const el = document.getElementById('events-info');

  if (!data || data.length === 0) {
    el.innerHTML = '<p class="empty">No recent events</p>';
    return;
  }

  let html = '';
  for (const ev of data) {
    html += `<div class="event-item">
      <span class="event-topic">${esc(ev.topic)}</span>
      <span class="event-time">#${ev.id}</span>
    </div>`;
  }
  el.innerHTML = html;
}

async function refresh() {
  await Promise.all([
    refreshTokenTax(),
    refreshHealth(),
    refreshInstances(),
    refreshState(),
    refreshEvents(),
  ]);
}

// Reset token tax counters.
document.getElementById('tt-reset').addEventListener('click', async () => {
  await fetch(API_BASE + '/api/metrics/reset', { method: 'POST' });
  refresh();
});

// Initial load + poll every 5 seconds.
refresh();
setInterval(refresh, 5000);
