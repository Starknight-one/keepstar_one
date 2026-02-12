const isDebug = () => {
  try { return localStorage.getItem('debug') === 'true'; } catch { return false; }
};

export const log = {
  debug(...args) { if (isDebug()) console.log('[keepstar]', ...args); },
  info(...args)  { if (isDebug()) console.info('[keepstar]', ...args); },
  warn(...args)  { console.warn('[keepstar]', ...args); },
  error(...args) { console.error('[keepstar]', ...args); },
  api(method, path, status, durationMs, error = null) {
    if (error) console.error('[keepstar:api]', method, path, status, `${durationMs}ms`, error);
    else if (isDebug()) console.log('[keepstar:api]', method, path, status, `${durationMs}ms`);
  },
};
