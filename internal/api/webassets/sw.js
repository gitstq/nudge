/* nudge service worker: offline app shell + Web Push display. */
const SHELL = ['./', './index.html', './app.js', './style.css', './icon.svg', './manifest.webmanifest'];
const CACHE = 'nudge-shell-v1';

self.addEventListener('install', (e) => {
  e.waitUntil(caches.open(CACHE).then((c) => c.addAll(SHELL)).then(() => self.skipWaiting()));
});

self.addEventListener('activate', (e) => {
  e.waitUntil(
    caches.keys().then((keys) => Promise.all(keys.filter((k) => k !== CACHE).map((k) => caches.delete(k))))
      .then(() => self.clients.claim())
  );
});

self.addEventListener('fetch', (e) => {
  const url = new URL(e.request.url);
  if (url.pathname.startsWith('/api/')) return; // never cache API/stream
  e.respondWith(
    caches.match(e.request).then((hit) => hit || fetch(e.request).then((res) => {
      const copy = res.clone();
      caches.open(CACHE).then((c) => c.put(e.request, copy)).catch(() => {});
      return res;
    }).catch(() => caches.match('./index.html')))
  );
});

self.addEventListener('push', (e) => {
  let data = {};
  try { data = e.data ? e.data.json() : {}; } catch { data = { title: 'nudge', body: e.data && e.data.text() }; }
  const levelIcon = { error: '🔴', warning: '🟡', success: '🟢' }[data.level] || '🔔';
  const options = {
    body: data.body || '',
    tag: data.id || 'nudge',
    icon: 'icon.svg',
    badge: 'icon.svg',
    data: { url: data.url ? data.url : './' },
    timestamp: data.created_at ? Date.parse(data.created_at) : Date.now()
  };
  e.waitUntil(self.registration.showNotification((data.title || data.topic || 'nudge') + '  ' + levelIcon, options));
  self.clients.matchAll().then((list) => list.forEach((c) => c.postMessage({ type: 'nudge', title: data.title })));
});

self.addEventListener('notificationclick', (e) => {
  e.notification.close();
  const target = e.notification.data?.url || './';
  e.waitUntil(self.clients.matchAll({ type: 'window', includeUncontrolled: true }).then((list) => {
    for (const c of list) { if ('focus' in c) { c.postMessage({ type:'focus' }); return c.focus(); } }
    return self.clients.openWindow(target);
  }));
});
