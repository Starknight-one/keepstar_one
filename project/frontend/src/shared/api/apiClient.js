import { log } from '../logger';

let _apiBaseUrl = import.meta.env.VITE_API_URL || 'http://localhost:8080/api/v1';
let _tenantSlug = null;

export function setTenantSlug(slug) {
  _tenantSlug = slug;
}

export function setApiBaseUrl(url) {
  _apiBaseUrl = url;
}

export function getHeaders() {
  const headers = { 'Content-Type': 'application/json' };
  if (_tenantSlug) {
    headers['X-Tenant-Slug'] = _tenantSlug;
  }
  return headers;
}

async function timedFetch(method, path, options = {}) {
  const start = performance.now();
  let status = 0;
  try {
    const response = await fetch(`${_apiBaseUrl}${path}`, { method, headers: getHeaders(), ...options });
    status = response.status;
    return response;
  } catch (err) {
    log.api(method, path, 0, Math.round(performance.now() - start), err);
    throw err;
  } finally {
    if (status) log.api(method, path, status, Math.round(performance.now() - start));
  }
}

export async function sendChatMessage(message, sessionId = null) {
  const body = { message };
  if (sessionId) {
    body.sessionId = sessionId;
  }

  const response = await timedFetch('POST', '/chat', { body: JSON.stringify(body) });

  if (!response.ok) {
    throw new Error(`API error: ${response.status}`);
  }

  return response.json();
}

export async function getSession(sessionId) {
  const response = await timedFetch('GET', `/session/${sessionId}`);

  if (response.status === 404) {
    return null; // Session not found or expired
  }

  if (!response.ok) {
    throw new Error(`API error: ${response.status}`);
  }

  return response.json();
}

// Catalog API

export async function getProducts(tenantSlug, filters = {}) {
  const params = new URLSearchParams();
  if (filters.category) params.set('category', filters.category);
  if (filters.brand) params.set('brand', filters.brand);
  if (filters.search) params.set('search', filters.search);
  if (filters.minPrice) params.set('minPrice', filters.minPrice);
  if (filters.maxPrice) params.set('maxPrice', filters.maxPrice);
  if (filters.limit) params.set('limit', filters.limit);
  if (filters.offset) params.set('offset', filters.offset);

  const queryString = params.toString();
  const path = `/tenants/${tenantSlug}/products${queryString ? '?' + queryString : ''}`;

  const response = await timedFetch('GET', path);

  if (!response.ok) {
    throw new Error(`API error: ${response.status}`);
  }

  return response.json();
}

export async function getProduct(tenantSlug, productId) {
  const response = await timedFetch('GET', `/tenants/${tenantSlug}/products/${productId}`);

  if (response.status === 404) {
    return null;
  }

  if (!response.ok) {
    throw new Error(`API error: ${response.status}`);
  }

  return response.json();
}

// Session init - creates session, resolves tenant, returns greeting
export async function initSession() {
  const response = await timedFetch('POST', '/session/init');

  if (!response.ok) {
    throw new Error(`API error: ${response.status}`);
  }

  // Response: { sessionId, tenant: { slug, name }, greeting }
  return response.json();
}

// Pipeline API - sends query through Agent 1 -> Agent 2 -> Formation
export async function sendPipelineQuery(sessionId, query) {
  const body = { query };
  if (sessionId) {
    body.sessionId = sessionId;
  }

  const response = await timedFetch('POST', '/pipeline', { body: JSON.stringify(body) });

  if (!response.ok) {
    throw new Error(`API error: ${response.status}`);
  }

  // Response: { sessionId, formation, agent1Ms, agent2Ms, totalMs }
  return response.json();
}

// Navigation API - expand widget to detail view
export async function expandView(sessionId, entityType, entityId) {
  const response = await timedFetch('POST', '/navigation/expand', {
    body: JSON.stringify({ sessionId, entityType, entityId }),
  });

  if (!response.ok) {
    throw new Error(`API error: ${response.status}`);
  }

  // Response: { success, formation, viewMode, focused, stackSize, canGoBack }
  return response.json();
}

// Navigation API - go back to previous view
export async function goBack(sessionId) {
  const response = await timedFetch('POST', '/navigation/back', {
    body: JSON.stringify({ sessionId }),
  });

  if (!response.ok) {
    throw new Error(`API error: ${response.status}`);
  }

  // Response: { success, formation, viewMode, focused, stackSize, canGoBack }
  return response.json();
}
