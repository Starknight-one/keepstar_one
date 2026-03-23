import React from 'react';
import { Link, useNavigate } from 'react-router-dom';
import './Header.css';

export default function Header() {
  const navigate = useNavigate();

  const handlePricingClick = (e) => {
    e.preventDefault();
    if (window.location.pathname !== '/') {
      navigate('/');
      setTimeout(() => {
        document.getElementById('pricing')?.scrollIntoView({ behavior: 'smooth' });
      }, 100);
    } else {
      document.getElementById('pricing')?.scrollIntoView({ behavior: 'smooth' });
    }
  };

  return (
    <header className="header container">
      <Link to="/" className="logo-wrap">
        <div className="logo-icon"></div>
        <span className="logo-text">Keepstar</span>
      </Link>

      <nav className="nav-links">
        <a href="#" onClick={(e) => { e.preventDefault(); }}>Product</a>
        <a href="#" onClick={(e) => { e.preventDefault(); }}>Use Cases</a>
        <a href="#pricing" onClick={handlePricingClick}>Pricing</a>
        <Link to="/blog">Blog</Link>
      </nav>

      <button className="btn-get-started">Get started</button>
    </header>
  );
}
