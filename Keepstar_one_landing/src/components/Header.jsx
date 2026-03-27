import React from 'react';
import { Link, useNavigate } from 'react-router-dom';
import './Header.css';

export default function Header({ onDemo }) {
  const navigate = useNavigate();

  const handleScrollTo = (id) => (e) => {
    e.preventDefault();
    if (window.location.pathname !== '/') {
      navigate('/');
      setTimeout(() => {
        document.getElementById(id)?.scrollIntoView({ behavior: 'smooth' });
      }, 100);
    } else {
      document.getElementById(id)?.scrollIntoView({ behavior: 'smooth' });
    }
  };

  return (
    <header className="header container">
      <Link to="/" className="logo-wrap">
        <div className="logo-icon"></div>
        <span className="logo-text">Keepstar One</span>
      </Link>

      <nav className="nav-links">
        <a href="#product" onClick={handleScrollTo('product')}>Product</a>
        <a href="#use-cases" onClick={handleScrollTo('use-cases')}>Use Cases</a>
        <a href="#pricing" onClick={handleScrollTo('pricing')}>Pricing</a>
        <Link to="/blog">Blog</Link>
      </nav>

      <button className="btn-get-started" onClick={onDemo}>Contact sales</button>
    </header>
  );
}
