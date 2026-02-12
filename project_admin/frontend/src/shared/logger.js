const isDebug = () => {
  try { return localStorage.getItem('debug') === 'true'; } catch { return false; }
};

export const log = {
  debug(...args) { if (isDebug()) console.log('[admin]', ...args); },
  info(...args)  { if (isDebug()) console.info('[admin]', ...args); },
  warn(...args)  { console.warn('[admin]', ...args); },
  error(...args) { console.error('[admin]', ...args); },
  api(method, path, status, durationMs, error = null) {
    if (error) console.error('[admin:api]', method, path, status, `${durationMs}ms`, error);
    else if (isDebug()) console.log('[admin:api]', method, path, status, `${durationMs}ms`);
  },
};
