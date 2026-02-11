let _apiBaseUrl = import.meta.env.VITE_API_URL || 'http://localhost:8080/api/v1';
let _tenantSlug = null;

export function setTenantSlug(slug) {
  _tenantSlug = slug;
}

export function setApiBaseUrl(url) {
  _apiBaseUrl = url;
}

function getHeaders() {
  const headers = { 'Content-Type': 'application/json' };
  if (_tenantSlug) {
    headers['X-Tenant-Slug'] = _tenantSlug;
  }
  return headers;
}

export async function sendChatMessage(message, sessionId = null) {
  const body = { message };
  if (sessionId) {
    body.sessionId = sessionId;
  }

  const response = await fetch(`${_apiBaseUrl}/chat`, {
    method: 'POST',
    headers: getHeaders(),
    body: JSON.stringify(body),
  });

  if (!response.ok) {
    throw new Error(`API error: ${response.status}`);
  }

  return response.json();
}

export async function getSession(sessionId) {
  const response = await fetch(`${_apiBaseUrl}/session/${sessionId}`, {
    method: 'GET',
    headers: getHeaders(),
  });

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
  const url = `${_apiBaseUrl}/tenants/${tenantSlug}/products${queryString ? '?' + queryString : ''}`;

  const response = await fetch(url, {
    method: 'GET',
    headers: getHeaders(),
  });

  if (!response.ok) {
    throw new Error(`API error: ${response.status}`);
  }

  return response.json();
}

export async function getProduct(tenantSlug, productId) {
  const response = await fetch(`${_apiBaseUrl}/tenants/${tenantSlug}/products/${productId}`, {
    method: 'GET',
    headers: getHeaders(),
  });

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
  const response = await fetch(`${_apiBaseUrl}/session/init`, {
    method: 'POST',
    headers: getHeaders(),
  });

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

  const response = await fetch(`${_apiBaseUrl}/pipeline`, {
    method: 'POST',
    headers: getHeaders(),
    body: JSON.stringify(body),
  });

  if (!response.ok) {
    throw new Error(`API error: ${response.status}`);
  }

  // Response: { sessionId, formation, agent1Ms, agent2Ms, totalMs }
  return response.json();
}

// Navigation API - expand widget to detail view
export async function expandView(sessionId, entityType, entityId) {
  const response = await fetch(`${_apiBaseUrl}/navigation/expand`, {
    method: 'POST',
    headers: getHeaders(),
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
  const response = await fetch(`${_apiBaseUrl}/navigation/back`, {
    method: 'POST',
    headers: getHeaders(),
    body: JSON.stringify({ sessionId }),
  });

  if (!response.ok) {
    throw new Error(`API error: ${response.status}`);
  }

  // Response: { success, formation, viewMode, focused, stackSize, canGoBack }
  return response.json();
}
