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

async function refreshMetrics() {
  const data = await fetchJSON('/api/metrics');
  const el = document.getElementById('metrics-info');

  if (!data) {
    el.innerHTML = '<p class="empty">No metrics available</p>';
    return;
  }

  const pairs = Object.entries(data).map(([k, v]) => [k, typeof v === 'object' ? JSON.stringify(v) : v]);
  el.innerHTML = renderTable(pairs);
}

async function refreshInstances() {
  const data = await fetchJSON('/api/instances');
  const el = document.getElementById('instances-info');

  if (!data || data.length === 0) {
    el.innerHTML = '<p class="empty">No instances registered</p>';
    return;
  }

  let html = '<table>';
  html += '<tr><td><strong>Name</strong></td><td><strong>Intent</strong></td></tr>';
  for (const inst of data) {
    html += `<tr><td>${esc(inst.name)}</td><td>${esc(inst.intent || '-')}</td></tr>`;
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
    refreshHealth(),
    refreshMetrics(),
    refreshInstances(),
    refreshState(),
    refreshEvents(),
  ]);
}

// Initial load + poll every 5 seconds.
refresh();
setInterval(refresh, 5000);
