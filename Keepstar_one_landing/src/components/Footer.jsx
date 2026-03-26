import React from 'react';
import { Link, useNavigate } from 'react-router-dom';
import { Twitter, Linkedin, Github } from 'lucide-react';
import './Footer.css';

export default function Footer() {
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
    <footer className="footer-full">
      <div className="footer-top">
        <div className="footer-brand">
          <Link to="/" className="logo-wrap">
            <div className="logo-icon"></div>
            <span className="logo-text">Keepstar</span>
          </Link>
          <p className="footer-brand-desc">
            AI-powered dynamic pages that replace static websites and turn every visitor into a conversion.
          </p>
        </div>

        <div className="footer-col">
          <span className="footer-col-title">Product</span>
          <Link to="/features">Features</Link>
          <a href="#pricing" onClick={handlePricingClick}>Pricing</a>
          <Link to="/changelog">Changelog</Link>
        </div>

        <div className="footer-col">
          <span className="footer-col-title">Company</span>
          <Link to="/about">About</Link>
          <Link to="/blog">Blog</Link>
          <Link to="/contact">Contact</Link>
        </div>

        <div className="footer-col">
          <span className="footer-col-title">Legal</span>
          <Link to="/privacy">Privacy Policy</Link>
          <Link to="/terms">Terms of Service</Link>
        </div>
      </div>

      <div className="footer-divider" />

      <div className="footer-bottom">
        <span className="footer-copyright">&copy; {new Date().getFullYear()} Keepstar. All rights reserved.</span>
        <div className="footer-social">
          <a href="#" aria-label="Twitter"><Twitter size={16} /></a>
          <a href="#" aria-label="LinkedIn"><Linkedin size={16} /></a>
          <a href="#" aria-label="GitHub"><Github size={16} /></a>
        </div>
      </div>
    </footer>
  );
}
