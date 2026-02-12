import { getHeaders } from '../../../shared/api/apiClient';
import { log } from '../../../shared/logger';

const API_BASE = import.meta.env.VITE_API_URL || 'http://localhost:8080/api/v1';

/**
 * Fire-and-forget sync of expand action to backend.
 * Frontend already rendered the formation from adjacentFormations cache;
 * this call keeps backend state in sync.
 */
export function syncExpand(sessionId, entityType, entityId) {
  fetch(`${API_BASE}/navigation/expand?sync=true`, {
    method: 'POST',
    headers: getHeaders(),
    body: JSON.stringify({ sessionId, entityType, entityId }),
    keepalive: true,
  }).catch((err) => log.warn('syncExpand failed:', err));
}

/**
 * Fire-and-forget sync of back action to backend.
 * Frontend already popped formation from stack;
 * this call keeps backend view stack in sync.
 */
export function syncBack(sessionId) {
  fetch(`${API_BASE}/navigation/back?sync=true`, {
    method: 'POST',
    headers: getHeaders(),
    body: JSON.stringify({ sessionId }),
    keepalive: true,
  }).catch((err) => log.warn('syncBack failed:', err));
}
