import React from 'react';
import { useNavigate } from 'react-router-dom';
import Header from '../components/Header';
import Footer from '../components/Footer';
import { BLOG_POSTS } from '../data/blogPosts';
import './BlogList.css';

const confettiShapes = [
  { type: 'ellipse', color: 'bg-blue', w: 10, top: '8%', left: '8%', delay: '' },
  { type: 'ellipse', color: 'bg-red', w: 7, top: '15%', left: '22%', delay: 'delay-100' },
  { type: 'rect', color: 'bg-yellow', w: 14, h: 5, top: '5%', left: '40%', rot: 'rotate(30deg)', delay: 'delay-200' },
  { type: 'ellipse', color: 'bg-green', w: 9, top: '18%', left: '55%', delay: '' },
  { type: 'ellipse', color: 'bg-blue', w: 6, top: '10%', left: '70%', delay: 'delay-100' },
  { type: 'rect', color: 'bg-red', w: 12, h: 5, top: '20%', left: '82%', rot: 'rotate(-20deg)', delay: 'delay-200' },
  { type: 'ellipse', color: 'bg-yellow', w: 8, top: '70%', left: '5%', delay: 'delay-300' },
  { type: 'rect', color: 'bg-green', w: 16, h: 5, top: '65%', left: '30%', rot: 'rotate(45deg)', delay: '' },
  { type: 'ellipse', color: 'bg-blue', w: 10, top: '75%', left: '60%', delay: 'delay-100' },
  { type: 'ellipse', color: 'bg-red', w: 7, top: '80%', left: '85%', delay: 'delay-200' },
  { type: 'rect', color: 'bg-yellow', w: 10, h: 5, top: '50%', left: '92%', rot: 'rotate(-35deg)', delay: 'delay-300' },
  { type: 'ellipse', color: 'bg-green', w: 8, top: '45%', left: '3%', delay: '' },
];

export default function BlogList() {
  const navigate = useNavigate();
  const featured = BLOG_POSTS.find((p) => p.featured);
  const rest = BLOG_POSTS.filter((p) => !p.featured);

  return (
    <div className="app-container">
      <Header />

      <section className="blog-hero">
        {confettiShapes.map((s, i) => (
          <div
            key={i}
            className={`floating-shape ${s.type === 'ellipse' ? 'shape-ellipse' : 'shape-rect'} ${s.color} ${s.delay}`}
            style={{
              top: s.top,
              left: s.left,
              width: s.w,
              height: s.h || s.w,
              transform: s.rot || 'none',
            }}
          />
        ))}
        <div className="blog-hero-content container">
          <div className="blog-badge">Blog</div>
          <h1 className="blog-hero-title">Insights & Updates</h1>
          <p className="blog-hero-subtitle">
            AI personalization, e-commerce strategy, and product updates from the Keepstar team.
          </p>
        </div>
      </section>

      <div className="container">
        {featured && (
          <div className="featured-post" onClick={() => navigate(`/blog/${featured.slug}`)}>
            <div className="featured-cover" style={{ background: featured.coverImage }} />
            <div className="featured-text">
              <span className="post-category">{featured.category}</span>
              <h2 className="featured-title">{featured.title}</h2>
              <p className="featured-excerpt">{featured.excerpt}</p>
              <div className="post-meta">
                <span>{featured.date}</span>
                <span>{featured.readTime}</span>
              </div>
            </div>
          </div>
        )}

        <div className="blog-grid">
          {rest.map((post) => (
            <div key={post.slug} className="blog-card" onClick={() => navigate(`/blog/${post.slug}`)}>
              <div className="blog-card-cover" style={{ background: post.coverImage }} />
              <div className="blog-card-body">
                <span className="post-category">{post.category}</span>
                <h3 className="blog-card-title">{post.title}</h3>
                <p className="blog-card-excerpt">{post.excerpt}</p>
                <div className="post-meta">
                  <span>{post.date}</span>
                  <span>{post.readTime}</span>
                </div>
              </div>
            </div>
          ))}
        </div>
      </div>

      <Footer />
    </div>
  );
}
