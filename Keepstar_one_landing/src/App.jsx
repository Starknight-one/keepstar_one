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
import TermsOfUse from './pages/TermsOfUse';
import PrivacyPolicy from './pages/PrivacyPolicy';
import Contact from './pages/Contact';
import About from './pages/About';
import FeaturesPage from './pages/FeaturesPage';
import ChangelogPage from './pages/ChangelogPage';
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
        <Route path="/terms" element={<TermsOfUse />} />
        <Route path="/privacy" element={<PrivacyPolicy />} />
        <Route path="/contact" element={<Contact />} />
        <Route path="/about" element={<About />} />
        <Route path="/features" element={<FeaturesPage />} />
        <Route path="/changelog" element={<ChangelogPage />} />
      </Routes>
      {showDemo && <DemoModal onClose={() => setShowDemo(false)} />}
    </>
  );
}

export default App;
