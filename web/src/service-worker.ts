/// <reference lib="webworker" />

import { build, files, version } from '$service-worker';

declare const self: ServiceWorkerGlobalScope;

const prefix = 'oib-app-';
const cacheName = prefix + version;
const shell = '/200.html';
const assets = new Set([...build, ...files, shell].filter((path) => !isApiPath(path)));

function isApiPath(path: string) {
  return path === '/api' || path.startsWith('/api/');
}

self.addEventListener('install', (event) => {
  event.waitUntil((async () => {
    const cache = await caches.open(cacheName);
    await cache.addAll([...assets].map((path) => new Request(path, { cache: 'reload' })));
  })());
});

self.addEventListener('activate', (event) => {
  event.waitUntil((async () => {
    const keys = await caches.keys();
    await Promise.all(keys
      .filter((key) => key.startsWith(prefix) && key !== cacheName)
      .map((key) => caches.delete(key)));
    await self.clients.claim();
  })());
});

self.addEventListener('fetch', (event) => {
  const { request } = event;
  const url = new URL(request.url);
  if (request.method !== 'GET' || url.origin !== self.location.origin || isApiPath(url.pathname)) return;

  if (assets.has(url.pathname)) {
    event.respondWith((async () => {
      const cache = await caches.open(cacheName);
      return (await cache.match(url.pathname)) ?? fetch(request);
    })());
  } else if (request.mode === 'navigate') {
    event.respondWith(fetch(request).catch(async () => {
      const cache = await caches.open(cacheName);
      return (await cache.match(shell)) ?? Response.error();
    }));
  }
});
