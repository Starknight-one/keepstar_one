import express from 'express';
import cors from 'cors';
import cookieParser from 'cookie-parser';
import jwt from 'jsonwebtoken';
import bcrypt from 'bcryptjs';
import Database from 'better-sqlite3';
import { fileURLToPath } from 'url';
import { dirname, join } from 'path';

const __dirname = dirname(fileURLToPath(import.meta.url));
const app = express();
const PORT = process.env.PORT || 3001;
const JWT_SECRET = process.env.JWT_SECRET || 'keepstar-blog-admin-secret-change-me';
const CORS_ORIGIN = process.env.CORS_ORIGIN || 'http://localhost:5173';

// --- Database ---
const db = new Database(join(__dirname, 'blog.db'));
db.pragma('journal_mode = WAL');
db.pragma('foreign_keys = ON');

db.exec(`
  CREATE TABLE IF NOT EXISTS users (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    email TEXT UNIQUE NOT NULL,
    password_hash TEXT NOT NULL,
    created_at TEXT DEFAULT (datetime('now'))
  );

  CREATE TABLE IF NOT EXISTS posts (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    title TEXT NOT NULL,
    slug TEXT UNIQUE NOT NULL,
    excerpt TEXT DEFAULT '',
    content TEXT DEFAULT '',
    category TEXT DEFAULT 'General',
    status TEXT DEFAULT 'draft' CHECK(status IN ('draft', 'published')),
    cover_image TEXT DEFAULT '',
    meta_description TEXT DEFAULT '',
    created_at TEXT DEFAULT (datetime('now')),
    updated_at TEXT DEFAULT (datetime('now'))
  );

  CREATE TABLE IF NOT EXISTS views (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    post_slug TEXT NOT NULL,
    viewed_at TEXT DEFAULT (datetime('now')),
    referrer TEXT DEFAULT ''
  );
`);

// Seed default admin if none exists
const userCount = db.prepare('SELECT COUNT(*) as c FROM users').get();
if (userCount.c === 0) {
  const hash = bcrypt.hashSync('admin123', 10);
  db.prepare('INSERT INTO users (email, password_hash) VALUES (?, ?)').run('admin@keepstar.one', hash);
  console.log('Default admin created: admin@keepstar.one / admin123');
}

// --- Middleware ---
app.use(cors({ origin: CORS_ORIGIN, credentials: true }));
app.use(express.json());
app.use(cookieParser());

function auth(req, res, next) {
  const token = req.cookies?.token || req.headers.authorization?.replace('Bearer ', '');
  if (!token) return res.status(401).json({ error: 'Unauthorized' });
  try {
    req.user = jwt.verify(token, JWT_SECRET);
    next();
  } catch {
    return res.status(401).json({ error: 'Invalid token' });
  }
}

// --- Auth ---
app.post('/api/auth/login', (req, res) => {
  const { email, password } = req.body;
  const user = db.prepare('SELECT * FROM users WHERE email = ?').get(email);
  if (!user || !bcrypt.compareSync(password, user.password_hash)) {
    return res.status(401).json({ error: 'Invalid credentials' });
  }
  const token = jwt.sign({ id: user.id, email: user.email }, JWT_SECRET, { expiresIn: '7d' });
  res.cookie('token', token, { httpOnly: true, maxAge: 7 * 24 * 60 * 60 * 1000, sameSite: 'lax' });
  res.json({ token, user: { id: user.id, email: user.email } });
});

app.post('/api/auth/logout', (_req, res) => {
  res.clearCookie('token');
  res.json({ ok: true });
});

app.get('/api/auth/me', auth, (req, res) => {
  res.json({ user: { id: req.user.id, email: req.user.email } });
});

// --- Posts (public) ---
app.get('/api/posts', (req, res) => {
  const { status } = req.query;
  let posts;
  if (status) {
    posts = db.prepare('SELECT * FROM posts WHERE status = ? ORDER BY created_at DESC').all(status);
  } else {
    posts = db.prepare('SELECT * FROM posts ORDER BY created_at DESC').all();
  }
  res.json(posts);
});

app.get('/api/posts/:slug', (req, res) => {
  const post = db.prepare('SELECT * FROM posts WHERE slug = ?').get(req.params.slug);
  if (!post) return res.status(404).json({ error: 'Post not found' });

  // Record view
  db.prepare('INSERT INTO views (post_slug, referrer) VALUES (?, ?)').run(
    req.params.slug,
    req.headers.referer || ''
  );

  res.json(post);
});

// --- Posts (admin) ---
app.post('/api/posts', auth, (req, res) => {
  const { title, slug, excerpt, content, category, status, cover_image, meta_description } = req.body;
  try {
    const result = db.prepare(
      'INSERT INTO posts (title, slug, excerpt, content, category, status, cover_image, meta_description) VALUES (?, ?, ?, ?, ?, ?, ?, ?)'
    ).run(title, slug, excerpt || '', content || '', category || 'General', status || 'draft', cover_image || '', meta_description || '');
    const post = db.prepare('SELECT * FROM posts WHERE id = ?').get(result.lastInsertRowid);
    res.status(201).json(post);
  } catch (err) {
    res.status(400).json({ error: err.message });
  }
});

app.put('/api/posts/:id', auth, (req, res) => {
  const { title, slug, excerpt, content, category, status, cover_image, meta_description } = req.body;
  try {
    db.prepare(
      `UPDATE posts SET title=?, slug=?, excerpt=?, content=?, category=?, status=?, cover_image=?, meta_description=?, updated_at=datetime('now') WHERE id=?`
    ).run(title, slug, excerpt, content, category, status, cover_image, meta_description, req.params.id);
    const post = db.prepare('SELECT * FROM posts WHERE id = ?').get(req.params.id);
    res.json(post);
  } catch (err) {
    res.status(400).json({ error: err.message });
  }
});

app.delete('/api/posts/:id', auth, (req, res) => {
  db.prepare('DELETE FROM posts WHERE id = ?').run(req.params.id);
  res.json({ ok: true });
});

// --- Analytics ---
app.get('/api/analytics', auth, (_req, res) => {
  const totalPosts = db.prepare('SELECT COUNT(*) as c FROM posts').get().c;
  const published = db.prepare("SELECT COUNT(*) as c FROM posts WHERE status='published'").get().c;
  const totalViews = db.prepare('SELECT COUNT(*) as c FROM views').get().c;
  const viewsToday = db.prepare("SELECT COUNT(*) as c FROM views WHERE viewed_at >= date('now')").get().c;

  const topPosts = db.prepare(`
    SELECT p.title, p.slug, COUNT(v.id) as views
    FROM posts p LEFT JOIN views v ON p.slug = v.post_slug
    GROUP BY p.slug ORDER BY views DESC LIMIT 5
  `).all();

  const topReferrers = db.prepare(`
    SELECT referrer, COUNT(*) as count FROM views
    WHERE referrer != '' GROUP BY referrer ORDER BY count DESC LIMIT 5
  `).all();

  const viewsByDay = db.prepare(`
    SELECT date(viewed_at) as day, COUNT(*) as count FROM views
    WHERE viewed_at >= date('now', '-30 days')
    GROUP BY day ORDER BY day
  `).all();

  res.json({ totalPosts, published, totalViews, viewsToday, topPosts, topReferrers, viewsByDay });
});

// --- Users (admin) ---
app.get('/api/users', auth, (_req, res) => {
  const users = db.prepare('SELECT id, email, created_at FROM users ORDER BY created_at DESC').all();
  res.json(users);
});

app.post('/api/users', auth, (req, res) => {
  const { email, password } = req.body;
  if (!email || !password) return res.status(400).json({ error: 'Email and password required' });
  try {
    const hash = bcrypt.hashSync(password, 10);
    const result = db.prepare('INSERT INTO users (email, password_hash) VALUES (?, ?)').run(email, hash);
    res.status(201).json({ id: result.lastInsertRowid, email });
  } catch (err) {
    res.status(400).json({ error: err.message });
  }
});

app.put('/api/users/:id/password', auth, (req, res) => {
  const { password } = req.body;
  if (!password) return res.status(400).json({ error: 'Password required' });
  const hash = bcrypt.hashSync(password, 10);
  db.prepare('UPDATE users SET password_hash = ? WHERE id = ?').run(hash, req.params.id);
  res.json({ ok: true });
});

app.delete('/api/users/:id', auth, (req, res) => {
  const count = db.prepare('SELECT COUNT(*) as c FROM users').get().c;
  if (count <= 1) return res.status(400).json({ error: 'Cannot delete the last admin user' });
  db.prepare('DELETE FROM users WHERE id = ?').run(req.params.id);
  res.json({ ok: true });
});

// --- Demo Requests ---
app.post('/api/demo-request', (req, res) => {
  console.log('Demo request received:', req.body);
  // TODO: store in DB + send email via Resend/SendGrid
  res.json({ ok: true });
});

// --- Serve static in production ---
if (process.env.NODE_ENV === 'production') {
  app.use(express.static(join(__dirname, 'dist')));
  app.get('*', (_req, res) => {
    res.sendFile(join(__dirname, 'dist', 'index.html'));
  });
}

app.listen(PORT, () => {
  console.log(`Blog Admin API running on http://localhost:${PORT}`);
});
