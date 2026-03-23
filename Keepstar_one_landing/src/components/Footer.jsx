import React from 'react';

export default function Footer() {
  return (
    <footer className="footer-simple container">
      <div className="footer-links-simple">
        <span>&copy; {new Date().getFullYear()} Keepstar One.</span>
        <a href="#">Privacy</a>
        <a href="#">Terms</a>
      </div>
    </footer>
  );
}
