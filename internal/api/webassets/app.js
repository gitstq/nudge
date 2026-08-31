'use strict';
/* nudge inbox SPA — vanilla JS, no dependencies, no CDN. */

const I18N = {
  en: {
    connecting: 'connecting…', connected: 'live', disconnected: 'offline',
    tab_inbox:'Inbox', tab_devices:'Devices', tab_keys:'Publish keys', tab_channels:'Channels', tab_settings:'Settings',
    logout:'Logout', login_title:'Connect to your nudge server', connect:'Connect',
    login_hint:'Enter the admin token printed on first boot (stored in data/admin.token). It stays in this browser only.',
    all_levels:'all levels', unread_only:'unread only', mark_all_read:'Mark all read', clear_all:'Clear all',
    inbox_empty:'No notifications yet — send one with curl!',
    this_device:'This device', device_hint:'Install this page as an app (Add to Home Screen / Install), then enable push. No native app and no APNs configuration required.',
    enable_push:'Enable push notifications', disable_push:'Disable on this device', enrolled:'Enrolled devices',
    new_key:'New publish key', create:'Create', token_once:'This token is shown only once — copy it now:', copy:'Copy',
    existing_keys:'Existing keys', new_channel:'New outbound channel', existing_channels:'Channels',
    quick_start:'Quick start', curl_hint:'Create a publish key in the previous tab, then:',
    stats:'Stats', unread:'unread', never:'never', test:'Test', delete:'Delete',
    push_unsupported:'Push messaging is not supported by this browser.',
    push_enabled:'Push enabled on this device.', push_blocked:'Notification permission denied.',
    confirm_clear:'Delete ALL events?', copied:'Copied'
  },
  zh: {
    connecting: '连接中…', connected: '实时在线', disconnected: '已断开',
    tab_inbox:'收件箱', tab_devices:'设备', tab_keys:'发布密钥', tab_channels:'外发通道', tab_settings:'设置',
    logout:'退出', login_title:'连接到你的 nudge 服务', connect:'连接',
    login_hint:'输入首次启动时打印的管理员令牌（保存在 data/admin.token）。它只会保存在当前浏览器。',
    all_levels:'全部级别', unread_only:'仅未读', mark_all_read:'全部标为已读', clear_all:'清空全部',
    inbox_empty:'还没有通知 —— 用 curl 发一条试试！',
    this_device:'当前设备', device_hint:'可先把本页“添加到主屏幕/安装为应用”，再开启推送。无需原生 App，也无需配置 APNs。',
    enable_push:'开启浏览器推送', disable_push:'关闭本设备推送', enrolled:'已注册设备',
    new_key:'新建发布密钥', create:'创建', token_once:'令牌只显示这一次，请立即复制：', copy:'复制',
    existing_keys:'已有密钥', new_channel:'新建外发通道', existing_channels:'通道列表',
    quick_start:'快速开始', curl_hint:'先在上一页创建发布密钥，然后执行：',
    stats:'统计', unread:'未读', never:'从未', test:'测试', delete:'删除',
    push_unsupported:'当前浏览器不支持 Web Push。',
    push_enabled:'本设备已开启推送。', push_blocked:'通知权限被拒绝。',
    confirm_clear:'确定清空所有事件？', copied:'已复制'
  }
};
let lang = localStorage.getItem('nudge-lang') || (navigator.language.startsWith('zh') ? 'zh' : 'en');
const t = (k) => (I18N[lang][k] || I18N.en[k] || k);
function applyI18n() {
  document.querySelectorAll('[data-i18n]').forEach(el => { el.textContent = t(el.dataset.i18n); });
}

const state = { token: localStorage.getItem('nudge-token') || '', vapid: '', events: [], evtSource: null, swReg: null };
const $ = (id) => document.getElementById(id);
const esc = (s) => String(s ?? '').replace(/[&<>"']/g, c => ({'&':'&amp;','<':'&lt;','>':'&gt;','"':'&quot;',"'":'&#39;'}[c]));

async function api(path, opts = {}) {
  opts.headers = Object.assign({ 'Authorization': 'Bearer ' + state.token, 'Content-Type': 'application/json' }, opts.headers || {});
  const res = await fetch(path, opts);
  if (res.status === 401) { showLogin(); throw new Error('unauthorized'); }
  const text = await res.text();
  const data = text ? JSON.parse(text) : {};
  if (!res.ok) throw new Error(data.error || res.statusText);
  return data;
}
function toast(msg) {
  const el = $('toast'); el.textContent = msg; el.classList.remove('hidden');
  clearTimeout(toast._t); toast._t = setTimeout(() => el.classList.add('hidden'), 2200);
}

// ---- navigation ----
document.querySelectorAll('#tabs button').forEach(b => b.addEventListener('click', () => {
  document.querySelectorAll('#tabs button').forEach(x => x.classList.toggle('active', x === b));
  document.querySelectorAll('.tabview').forEach(v => v.classList.add('hidden'));
  $('tab-' + b.dataset.tab).classList.remove('hidden');
  refreshTab(b.dataset.tab);
}));
$('lang-toggle').addEventListener('click', () => { lang = lang === 'en' ? 'zh' : 'en'; localStorage.setItem('nudge-lang', lang); applyI18n(); });
$('logout').addEventListener('click', () => { localStorage.removeItem('nudge-token'); location.reload(); });

// ---- login ----
function showLogin() {
  $('login-view').classList.remove('hidden');
  document.querySelectorAll('.tabview').forEach(v => v.classList.add('hidden'));
  setConn(false, 'disconnected');
}
$('login-btn').addEventListener('click', login);
$('admin-token').addEventListener('keydown', e => { if (e.key === 'Enter') login(); });
async function login() {
  state.token = $('admin-token').value.trim();
  try {
    await api('/api/v1/stats');
    localStorage.setItem('nudge-token', state.token);
    await boot();
  } catch (e) { $('login-err').textContent = e.message; }
}
function setConn(ok, key) {
  const c = $('conn'); c.textContent = t(key); c.className = 'conn ' + (ok ? 'ok' : 'bad');
}

// ---- boot ----
async function boot() {
  $('login-view').classList.add('hidden');
  $('tab-inbox').classList.remove('hidden');
  applyI18n();
  state.vapid = (await api('/api/v1/vapid-public')).public_key;
  await Promise.all([loadEvents(), refreshTab('devices'), refreshTab('keys'), refreshTab('channels'), refreshTab('settings')]);
  connectStream();
  if ('serviceWorker' in navigator) navigator.serviceWorker.register('sw.js').then(r => { state.swReg = r; }).catch(()=>{});
}
function connectStream() {
  if (state.evtSource) state.evtSource.close();
  const es = new EventSource('/api/v1/stream?token=' + encodeURIComponent(state.token));
  state.evtSource = es;
  es.onopen = () => setConn(true, 'connected');
  es.onerror = () => setConn(false, 'disconnected');
  es.addEventListener('message', ev => {
    const e = JSON.parse(ev.data);
    state.events.unshift(e);
    renderEvents();
    if (document.hidden && 'Notification' in window && Notification.permission === 'granted') {
      new Notification(e.title || e.topic, { body: e.body, tag: e.id, icon: 'icon.svg' });
    }
  });
}

// ---- inbox ----
async function loadEvents() {
  const topic = $('filter-topic').value.trim();
  const unread = $('filter-unread').checked;
  const level = $('filter-level').value;
  const q = new URLSearchParams({ limit: '200' });
  if (topic) q.set('topic', topic);
  if (unread) q.set('unread', '1');
  const data = await api('/api/v1/events?' + q);
  state.events = data.events.filter(e => !level || e.level === level);
  renderEvents();
}
function renderEvents() {
  const list = $('event-list'); list.innerHTML = '';
  $('inbox-empty').classList.toggle('hidden', state.events.length > 0);
  let unread = 0;
  for (const e of state.events) {
    if (!e.read) unread++;
    const div = document.createElement('div');
    div.className = 'event level-' + e.level + (e.read ? ' read' : '');
    const tags = (e.tags || []).map(x => `<span class="tag">${esc(x)}</span>`).join(' ');
    div.innerHTML = `
      <div>
        <div class="t">${esc(e.title || e.topic)}</div>
        <div class="b">${esc(e.body)}</div>
        <div class="meta"><span>#${esc(e.topic)}</span><span>${new Date(e.created_at).toLocaleString()}</span>${tags}
          ${e.url ? `<a href="${esc(e.url)}" target="_blank" rel="noopener">↗ link</a>` : ''}</div>
      </div>
      <div class="acts">
        ${e.read ? '' : '<button class="ghost" data-act="read">✓</button>'}
        <button class="ghost danger" data-act="del">✕</button>
      </div>`;
    div.querySelector('[data-act="del"]').addEventListener('click', async () => {
      await api('/api/v1/events/' + e.id, { method: 'DELETE' });
      state.events = state.events.filter(x => x.id !== e.id); renderEvents();
    });
    const rb = div.querySelector('[data-act="read"]');
    if (rb) rb.addEventListener('click', async () => {
      await api('/api/v1/events/read', { method:'POST', body: JSON.stringify({ ids:[e.id] }) });
      e.read = true; renderEvents();
    });
    list.appendChild(div);
  }
  $('unread-badge').textContent = `${unread} ${t('unread')}`;
}
['filter-topic','filter-level','filter-unread'].forEach(id => $(id).addEventListener('input', loadEvents));
$('btn-read-all').addEventListener('click', async () => { await api('/api/v1/events/read',{method:'POST',body:JSON.stringify({all:true})}); loadEvents(); });
$('btn-clear').addEventListener('click', async () => { if (confirm(t('confirm_clear'))) { await api('/api/v1/events/clear',{method:'POST'}); loadEvents(); } });

// ---- devices ----
function urlB64ToUint8Array(base64) {
  const pad = '='.repeat((4 - base64.length % 4) % 4);
  const b = atob(base64.replace(/-/g,'+').replace(/_/g,'/') + pad);
  return Uint8Array.from(b, c => c.charCodeAt(0));
}
$('btn-enroll').addEventListener('click', async () => {
  const msg = $('enroll-msg');
  if (!('serviceWorker' in navigator) || !('PushManager' in window)) { msg.textContent = t('push_unsupported'); return; }
  const perm = await Notification.requestPermission();
  if (perm !== 'granted') { msg.textContent = t('push_blocked'); return; }
  const reg = state.swReg || await navigator.serviceWorker.ready;
  const sub = await reg.pushManager.subscribe({ userVisibleOnly: true, applicationServerKey: urlB64ToUint8Array(state.vapid) });
  const j = sub.toJSON();
  await api('/api/v1/devices', { method:'POST', body: JSON.stringify({ name: navigator.userAgent.slice(0,60), endpoint:j.endpoint, p256dh:j.keys.p256dh, auth:j.keys.auth }) });
  msg.textContent = t('push_enabled'); toast(t('push_enabled'));
  refreshTab('devices');
});
$('btn-unenroll').addEventListener('click', async () => {
  const reg = state.swReg || await navigator.serviceWorker.ready;
  const sub = await reg.pushManager.getSubscription();
  if (sub) await sub.unsubscribe();
  refreshTab('devices');
});
async function renderDevices() {
  const data = await api('/api/v1/devices');
  const tb = $('device-rows'); tb.innerHTML = '';
  let mine = false;
  if (state.swReg) { const s = await state.swReg.pushManager?.getSubscription?.(); mine = !!s; }
  $('btn-unenroll').classList.toggle('hidden', !mine);
  for (const d of data.devices) {
    const tr = document.createElement('tr');
    const host = (() => { try { return new URL(d.endpoint).host; } catch { return d.endpoint; } })();
    tr.innerHTML = `<td>${esc(d.name||'-')}</td><td><code>${esc(host)}</code></td>
      <td>${d.last_status ? esc(d.last_status) + (d.last_error ? ' · '+esc(d.last_error) : '') : t('never')}</td>
      <td><button class="ghost danger">${t('delete')}</button></td>`;
    tr.querySelector('button').addEventListener('click', async () => { await api('/api/v1/devices/'+d.id,{method:'DELETE'}); renderDevices(); });
    tb.appendChild(tr);
  }
}

// ---- keys ----
$('btn-add-key').addEventListener('click', async () => {
  const data = await api('/api/v1/keys', { method:'POST', body: JSON.stringify({ name:$('key-name').value, topic:$('key-topic').value }) });
  $('key-result').classList.remove('hidden'); $('key-token').textContent = data.token;
  $('key-name').value = ''; $('key-topic').value = '';
  renderKeys();
});
$('copy-key').addEventListener('click', () => { navigator.clipboard.writeText($('key-token').textContent); toast(t('copied')); });
async function renderKeys() {
  const data = await api('/api/v1/keys');
  const tb = $('key-rows'); tb.innerHTML = '';
  for (const k of data.keys) {
    const tr = document.createElement('tr');
    tr.innerHTML = `<td>${esc(k.name)}</td><td><code>${esc(k.prefix)}</code></td><td>${esc(k.topic||'*')}</td>
      <td>${new Date(k.created_at).toLocaleDateString()}</td><td><button class="ghost danger">${t('delete')}</button></td>`;
    tr.querySelector('button').addEventListener('click', async () => { await api('/api/v1/keys/'+k.id,{method:'DELETE'}); renderKeys(); });
    tb.appendChild(tr);
  }
}

// ---- channels ----
$('btn-add-ch').addEventListener('click', async () => {
  await api('/api/v1/channels', { method:'POST', body: JSON.stringify({
    type:$('ch-type').value, name:$('ch-name').value, target:$('ch-target').value,
    topics:$('ch-topics').value.split(',').map(s=>s.trim()).filter(Boolean) }) });
  $('ch-name').value=''; $('ch-target').value=''; $('ch-topics').value='';
  renderChannels();
});
async function renderChannels() {
  const data = await api('/api/v1/channels');
  const tb = $('channel-rows'); tb.innerHTML = '';
  for (const c of data.channels) {
    const tr = document.createElement('tr');
    tr.innerHTML = `<td>${esc(c.name)}</td><td>${esc(c.type)}</td><td><code>${esc(c.target)}</code></td>
      <td>${c.last_ok_at ? '✓ '+new Date(c.last_ok_at).toLocaleString() : (c.last_error?esc(c.last_error):t('never'))}</td>
      <td><button class="ghost" data-a="test">${t('test')}</button> <button class="ghost danger" data-a="del">${t('delete')}</button></td>`;
    tr.querySelector('[data-a=test]').addEventListener('click', async b => {
      try { await api('/api/v1/channels/'+c.id+'/test',{method:'POST'}); toast('OK'); } catch(e){ toast(e.message); }
    });
    tr.querySelector('[data-a=del]').addEventListener('click', async () => { await api('/api/v1/channels/'+c.id,{method:'DELETE'}); renderChannels(); });
    tb.appendChild(tr);
  }
}

// ---- settings ----
async function renderSettings() {
  $('vapid-pub').textContent = state.vapid;
  $('curl-sample').textContent =
    `curl -X POST ${location.origin}/api/v1/notify \\\n` +
    `  -H "Authorization: Bearer <PUBLISH_KEY>" \\\n` +
    `  -H "Content-Type: application/json" \\\n` +
    `  -d '{"topic":"backups","title":"Backup OK","body":"nightly backup finished","level":"success"}'`;
  $('stats-box').textContent = JSON.stringify(await api('/api/v1/stats'), null, 2);
}

async function refreshTab(name) {
  if (!state.token) return;
  try {
    if (name === 'inbox') await loadEvents();
    if (name === 'devices') await renderDevices();
    if (name === 'keys') await renderKeys();
    if (name === 'channels') await renderChannels();
    if (name === 'settings') await renderSettings();
  } catch (e) { /* transient; stream/refresh will retry */ }
}

// Service worker messages for foreground fallback toast.
if ('serviceWorker' in navigator) {
  navigator.serviceWorker.addEventListener('message', ev => { if (ev.data?.type === 'nudge') toast(ev.data.title); });
}

applyI18n();
if (state.token) boot().catch(showLogin); else showLogin();
