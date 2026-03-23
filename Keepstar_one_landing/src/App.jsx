import React from 'react';
import Header from './components/Header';
import Hero from './components/Hero';
import ProductDemo from './components/ProductDemo';
import IconStrip from './components/IconStrip';
import FeatureRows from './components/FeatureRows';
import ProblemSection from './components/ProblemSection';
import HowItWorks from './components/HowItWorks';
import Stats from './components/Stats';
import FinalCTA from './components/FinalCTA';
import './App.css';

function App() {
  return (
    <div className="app-container">
      <Header />
      <Hero />
      <ProductDemo />
      <IconStrip />
      <FeatureRows />
      <ProblemSection />
      <HowItWorks />
      <Stats />
      <FinalCTA />
      
      <footer className="footer-simple container">
        <div className="footer-links-simple">
          <span>&copy; {new Date().getFullYear()} Keepstar One.</span>
          <a href="#">Privacy</a>
          <a href="#">Terms</a>
        </div>
      </footer>
    </div>
  );
}

export default App;
