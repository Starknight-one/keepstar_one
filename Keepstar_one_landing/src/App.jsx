import React, { useState } from 'react';
import { Routes, Route } from 'react-router-dom';
import Header from './components/Header';
import Hero from './components/Hero';
import ProductDemo from './components/ProductDemo';
import IconStrip from './components/IconStrip';
import FeatureRows from './components/FeatureRows';
import ProblemSection from './components/ProblemSection';
import HowItWorks from './components/HowItWorks';
import Stats from './components/Stats';
import Pricing from './components/Pricing';
import FinalCTA from './components/FinalCTA';
import Footer from './components/Footer';
import DemoModal from './components/DemoModal';
import BlogList from './pages/BlogList';
import BlogArticle from './pages/BlogArticle';
import './App.css';

function Landing({ onDemo }) {
  return (
    <div className="app-container">
      <Header onDemo={onDemo} />
      <Hero onDemo={onDemo} />
      <ProductDemo />
      <IconStrip />
      <FeatureRows />
      <ProblemSection />
      <HowItWorks />
      <Stats />
      <Pricing onDemo={onDemo} />
      <FinalCTA onDemo={onDemo} />
      <Footer />
    </div>
  );
}

function App() {
  const [showDemo, setShowDemo] = useState(false);

  return (
    <>
      <Routes>
        <Route path="/" element={<Landing onDemo={() => setShowDemo(true)} />} />
        <Route path="/blog" element={<BlogList />} />
        <Route path="/blog/:slug" element={<BlogArticle />} />
      </Routes>
      {showDemo && <DemoModal onClose={() => setShowDemo(false)} />}
    </>
  );
}

export default App;
