const CACHE_KEY = 'chatSessionCache';

/**
 * Save session state to localStorage for instant restore on next visit.
 * Stores: sessionId, messages (without formation blobs), last formation.
 */
export function saveSessionCache({ sessionId, messages, formation }) {
  if (!sessionId) return;
  try {
    // Strip formation from messages to save space — we store last formation separately
    // eslint-disable-next-line no-unused-vars
    const lightMessages = messages.map(({ formation, ...msg }) => msg);
    const data = {
      sessionId,
      messages: lightMessages,
      formation: formation || null,
      savedAt: Date.now(),
    };
    localStorage.setItem(CACHE_KEY, JSON.stringify(data));
  } catch {
    // localStorage full or unavailable — ignore
  }
}

/**
 * Load cached session state. Returns null if no cache or expired (>30 min).
 */
export function loadSessionCache() {
  try {
    const raw = localStorage.getItem(CACHE_KEY);
    if (!raw) return null;
    const data = JSON.parse(raw);
    // Expire after 30 minutes
    if (Date.now() - data.savedAt > 30 * 60 * 1000) {
      clearSessionCache();
      return null;
    }
    return data;
  } catch {
    return null;
  }
}

/**
 * Clear the session cache (on kill session, logout, etc.)
 */
export function clearSessionCache() {
  localStorage.removeItem(CACHE_KEY);
  localStorage.removeItem('chatSessionId');
}
