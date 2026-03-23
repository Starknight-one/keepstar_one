const BASE = '';

async function request(path, options = {}) {
  const res = await fetch(`${BASE}${path}`, {
    credentials: 'include',
    headers: { 'Content-Type': 'application/json', ...options.headers },
    ...options,
  });
  if (res.status === 401 && !path.includes('/auth/')) {
    window.location.href = '/login';
    return;
  }
  const data = await res.json();
  if (!res.ok) throw new Error(data.error || 'Request failed');
  return data;
}

export const api = {
  // Auth
  login: (email, password) => request('/api/auth/login', { method: 'POST', body: JSON.stringify({ email, password }) }),
  logout: () => request('/api/auth/logout', { method: 'POST' }),
  me: () => request('/api/auth/me'),

  // Posts
  getPosts: () => request('/api/posts'),
  getPost: (slug) => request(`/api/posts/${slug}`),
  createPost: (data) => request('/api/posts', { method: 'POST', body: JSON.stringify(data) }),
  updatePost: (id, data) => request(`/api/posts/${id}`, { method: 'PUT', body: JSON.stringify(data) }),
  deletePost: (id) => request(`/api/posts/${id}`, { method: 'DELETE' }),

  // Analytics
  getAnalytics: () => request('/api/analytics'),

  // Users
  getUsers: () => request('/api/users'),
  createUser: (email, password) => request('/api/users', { method: 'POST', body: JSON.stringify({ email, password }) }),
  changePassword: (id, password) => request(`/api/users/${id}/password`, { method: 'PUT', body: JSON.stringify({ password }) }),
  deleteUser: (id) => request(`/api/users/${id}`, { method: 'DELETE' }),
};
