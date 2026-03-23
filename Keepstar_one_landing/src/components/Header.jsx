import React from 'react';
import './Header.css';

export default function Header() {
  return (
    <header className="header container">
      <div className="logo-wrap">
        <div className="logo-icon"></div>
        <span className="logo-text">Keepstar</span>
      </div>
      
      <nav className="nav-links">
        <a href="#">Product</a>
        <a href="#">Use Cases</a>
        <a href="#">Pricing</a>
        <a href="#">Blog</a>
      </nav>
      
      <button className="btn-get-started">Get started</button>
    </header>
  );
}
