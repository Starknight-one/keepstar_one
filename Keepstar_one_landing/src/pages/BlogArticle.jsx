import React, { useState } from 'react';
import { useParams, useNavigate } from 'react-router-dom';
import { ArrowLeft } from 'lucide-react';
import Header from '../components/Header';
import Footer from '../components/Footer';
import DemoModal from '../components/DemoModal';
import { BLOG_POSTS } from '../data/blogPosts';
import './BlogArticle.css';

const bulletColors = ['#4285F4', '#EA4335', '#FBBC04', '#34A853'];

export default function BlogArticle() {
  const { slug } = useParams();
  const navigate = useNavigate();
  const [showDemo, setShowDemo] = useState(false);

  const post = BLOG_POSTS.find((p) => p.slug === slug);
  const related = BLOG_POSTS.filter((p) => p.slug !== slug).slice(0, 3);

  if (!post) {
    return (
      <div className="app-container">
        <Header />
        <div className="article-container" style={{ paddingTop: 120, textAlign: 'center' }}>
          <h1>Post not found</h1>
          <p style={{ marginTop: 16 }}>
            <span className="back-link" onClick={() => navigate('/blog')}>
              <ArrowLeft size={18} /> Back to Blog
            </span>
          </p>
        </div>
        <Footer />
      </div>
    );
  }

  return (
    <div className="app-container">
      <Header />

      <div className="article-hero-image" style={{ background: post.coverImage }} />

      <div className="article-container">
        <span className="back-link" onClick={() => navigate('/blog')}>
          <ArrowLeft size={18} /> Back to Blog
        </span>

        <div className="article-meta">
          <span className="article-category">{post.category}</span>
          <span className="article-date">{post.date}</span>
          <span className="article-readtime">{post.readTime}</span>
        </div>

        <h1 className="article-title">{post.title}</h1>
        <p className="article-intro">{post.body.intro}</p>

        <div className="article-body">
          {post.body.sections.map((section, i) => {
            if (section.type === 'heading') {
              return <h2 key={i}>{section.content}</h2>;
            }
            if (section.type === 'paragraph') {
              return <p key={i}>{section.content}</p>;
            }
            if (section.type === 'list') {
              return (
                <ul key={i}>
                  {section.items.map((item, j) => (
                    <li key={j}>
                      <span className="bullet-dot" style={{ background: bulletColors[j % 4] }} />
                      {item}
                    </li>
                  ))}
                </ul>
              );
            }
            return null;
          })}
        </div>

        <div className="article-cta">
          <h3>See it in action</h3>
          <p>Book a personalized demo and see how Keepstar One can work for your store.</p>
          <button className="btn-article-cta" onClick={() => setShowDemo(true)}>
            Book a Demo
          </button>
        </div>
      </div>

      <div className="related-section container">
        <h2 className="related-title">Related Posts</h2>
        <div className="blog-grid">
          {related.map((p) => (
            <div key={p.slug} className="blog-card" onClick={() => navigate(`/blog/${p.slug}`)}>
              <div className="blog-card-cover" style={{ background: p.coverImage }} />
              <div className="blog-card-body">
                <span className="post-category">{p.category}</span>
                <h3 className="blog-card-title">{p.title}</h3>
                <div className="post-meta">
                  <span>{p.date}</span>
                  <span>{p.readTime}</span>
                </div>
              </div>
            </div>
          ))}
        </div>
      </div>

      <Footer />

      {showDemo && <DemoModal onClose={() => setShowDemo(false)} />}
    </div>
  );
}
