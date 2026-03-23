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

// Seed posts if empty
const postCount = db.prepare('SELECT COUNT(*) as c FROM posts').get();
if (postCount.c === 0) {
  const seedPosts = [
    {
      title: 'How AI Personalization Increases E-Commerce Conversion by 340%',
      slug: 'how-ai-personalization-increases-conversion',
      excerpt: 'Static websites treat every visitor the same. Learn how dynamic AI-generated pages create unique experiences that dramatically boost conversion rates.',
      content: `E-commerce has a conversion problem. The average online store converts just 2-3% of visitors — meaning 97% of your traffic leaves without buying. The root cause? Every visitor sees the exact same page, regardless of their intent, preferences, or stage in the buying journey.

## The Static Website Problem

Traditional websites are built once and served to everyone. A first-time visitor researching skincare sees the same homepage as a returning customer ready to buy their favorite moisturizer. This one-size-fits-all approach creates friction at every stage of the funnel.

- New visitors get overwhelmed by too many product options
- Returning customers have to re-navigate to products they already know
- High-intent buyers see generic messaging instead of conversion-focused content
- Different demographics receive identical visual layouts and copy

## How Dynamic AI Pages Work

Keepstar One generates a unique page for every visitor in real-time. Our AI chat interface understands visitor intent through natural conversation, then assembles personalized product displays, comparisons, and detailed views — all without any manual configuration from the store owner.

## Results From Early Adopters

Our beta customers have seen remarkable improvements across key metrics. Average session duration increased by 4.2x, add-to-cart rates jumped 280%, and overall conversion rates improved by 340% compared to their static storefronts.

- 340% increase in conversion rate
- 4.2x longer average session duration
- 280% higher add-to-cart rate
- 67% reduction in bounce rate

## Getting Started

Integration takes less than 5 minutes — just add a single script tag to your website. Keepstar One handles the rest, from AI model training on your product catalog to real-time page generation for every visitor.`,
      category: 'Case Study',
      status: 'published',
      cover_image: 'linear-gradient(135deg, #4285F4 0%, #1A73E8 100%)',
      meta_description: 'Learn how AI personalization increases e-commerce conversion rates by 340%.',
    },
    {
      title: 'Visual Assembly Engine: How We Build UI in Real-Time',
      slug: 'visual-assembly-engine-behind-the-scenes',
      excerpt: 'A deep dive into the technology that powers Keepstar One — generating production-ready product displays from a single chat message.',
      content: `Behind every personalized page Keepstar One generates is the Visual Assembly Engine — a constraint-based system that transforms raw product data into polished, responsive UI components in milliseconds.

## The Challenge of Dynamic UI

Generating UI at runtime is fundamentally different from designing static templates. The system must handle variable data shapes, enforce visual hierarchy, maintain accessibility, and produce consistent results — all without human oversight.

## Three-Layer Architecture

The engine uses a three-layer hierarchy: Formations (layout containers), Widgets (individual cards or blocks), and Atoms (primitive data elements). Each layer has its own set of constraints and defaults that cascade downward.

- Formations handle layout: grid, list, carousel, comparison, table
- Widgets manage card structure: size, template, visual style
- Atoms render data: text styles, number formats, image treatments
- 30+ constraint rules ensure visual consistency across any data shape

## AI-Driven Assembly

A two-agent LLM pipeline powers the assembly process. Agent 1 understands visitor intent and fetches relevant data. Agent 2 selects presets, applies overrides, and calls the Visual Assembly Engine to produce the final formation. The entire process takes under 2 seconds.`,
      category: 'Engineering',
      status: 'published',
      cover_image: 'linear-gradient(135deg, #34A853 0%, #1A8A3E 100%)',
      meta_description: 'Deep dive into the Visual Assembly Engine powering Keepstar One.',
    },
    {
      title: 'Why Chat-First Commerce Outperforms Traditional Search',
      slug: 'ecommerce-chat-vs-traditional-search',
      excerpt: "Product search is broken. Filters and keywords fail when shoppers don't know exactly what they want. Here's why conversational commerce is the answer.",
      content: `When was the last time you used a product filter and actually found what you were looking for on the first try? Traditional e-commerce search relies on customers knowing exactly what they want — but most shoppers are browsing, exploring, or looking for recommendations.

## The Search Bar Failure

Studies show that 72% of e-commerce search queries return irrelevant results. Faceted navigation helps, but forces customers to translate their needs into predefined categories — "I need something for dry skin that's not too expensive" becomes a complex filter combination that most users abandon.

## Conversational Discovery

Chat-first commerce flips the paradigm. Instead of forcing customers to adapt to your navigation, the AI adapts to how customers naturally express their needs. A visitor can say "I need a gift for my mom who loves gardening" and get personalized recommendations instantly.

- Natural language understanding handles vague and complex queries
- Context builds over the conversation — preferences are remembered
- Visual responses show products in context, not just a list
- Follow-up questions narrow results without starting over

## The Hybrid Approach

Keepstar One doesn't replace your existing store — it enhances it. The chat widget sits alongside your traditional navigation, catching visitors who would otherwise bounce and guiding them to the right products through conversation.`,
      category: 'Product',
      status: 'published',
      cover_image: 'linear-gradient(135deg, #EA4335 0%, #C5221F 100%)',
      meta_description: 'Why conversational commerce outperforms traditional product search.',
    },
    {
      title: 'From Zero to AI-Powered Store in 5 Minutes',
      slug: 'five-minute-integration-guide',
      excerpt: 'A step-by-step guide to adding Keepstar One to your e-commerce site. One script tag, zero configuration, instant results.',
      content: `Adding AI-powered personalization to your store shouldn't require a team of engineers or months of integration work. With Keepstar One, you can go from zero to a fully functional AI assistant in under 5 minutes.

## Step 1: Sign Up and Connect Your Catalog

Create your Keepstar One account and point it at your product catalog. We support direct integrations with Shopify, WooCommerce, and BigCommerce, or you can upload a JSON feed. Our crawler can also automatically extract products from your existing site.

## Step 2: Add the Script Tag

Copy a single line of code and paste it into your website's HTML. The script automatically loads the chat widget, styled to match your brand colors. No CSS overrides, no configuration files — it just works.

## Step 3: Watch It Learn

Keepstar One immediately begins processing your catalog — enriching product data with AI-extracted attributes, building semantic search indexes, and learning the relationships between your products. Within minutes, visitors can start chatting and getting personalized recommendations.

- Automatic catalog enrichment with AI-extracted attributes
- Semantic search index built in real-time
- Brand-matched widget styling
- Analytics dashboard from day one`,
      category: 'Tutorial',
      status: 'published',
      cover_image: 'linear-gradient(135deg, #FBBC04 0%, #F29900 100%)',
      meta_description: 'Step-by-step guide to adding AI-powered personalization to your store.',
    },
  ];

  const insert = db.prepare(
    'INSERT INTO posts (title, slug, excerpt, content, category, status, cover_image, meta_description) VALUES (?, ?, ?, ?, ?, ?, ?, ?)'
  );
  for (const p of seedPosts) {
    insert.run(p.title, p.slug, p.excerpt, p.content, p.category, p.status, p.cover_image, p.meta_description);
  }
  console.log('Seeded 4 blog posts');
}

// --- Middleware ---
app.use(cors({ origin: true, credentials: true }));
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
  app.get('{*path}', (_req, res) => {
    res.sendFile(join(__dirname, 'dist', 'index.html'));
  });
}

app.listen(PORT, () => {
  console.log(`Blog Admin API running on http://localhost:${PORT}`);
});
