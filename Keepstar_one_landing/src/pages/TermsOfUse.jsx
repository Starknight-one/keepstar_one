import React, { useEffect } from 'react';
import Header from '../components/Header';
import Footer from '../components/Footer';
import './StaticPage.css';

const confetti = [
  { type: 'ellipse', color: 'bg-blue', w: 6, top: '20%', left: '6%', delay: '' },
  { type: 'ellipse', color: 'bg-red', w: 4, top: '13%', left: '18%', delay: 'delay-100' },
  { type: 'rect', color: 'bg-yellow', w: 5, h: 3, top: '35%', left: '30%', rot: '25deg', delay: 'delay-200' },
  { type: 'ellipse', color: 'bg-green', w: 3, top: '10%', left: '43%', delay: '' },
  { type: 'ellipse', color: 'bg-blue', w: 3, top: '28%', left: '56%', delay: 'delay-100' },
  { type: 'ellipse', color: 'bg-red', w: 7, top: '17%', left: '68%', delay: 'delay-200' },
  { type: 'rect', color: 'bg-yellow', w: 7, h: 3, top: '38%', left: '79%', rot: '40deg', delay: 'delay-300' },
  { type: 'ellipse', color: 'bg-green', w: 5, top: '14%', left: '90%', delay: '' },
  { type: 'rect', color: 'bg-blue', w: 9, h: 3, top: '82%', left: '25%', rot: '-15deg', delay: 'delay-100' },
  { type: 'ellipse', color: 'bg-yellow', w: 4, top: '90%', left: '50%', delay: 'delay-200' },
  { type: 'rect', color: 'bg-red', w: 6, h: 3, top: '76%', left: '75%', rot: '55deg', delay: 'delay-300' },
  { type: 'ellipse', color: 'bg-green', w: 4, top: '96%', left: '11%', delay: '' },
];

const sections = [
  { title: '1. Acceptance of Terms', text: 'By accessing or using Keepstar\'s services, website, or any associated applications (collectively, the "Service"), you agree to be bound by these Terms of Use. If you do not agree to all of these terms, do not use the Service. We reserve the right to update these terms at any time, and your continued use of the Service constitutes acceptance of any changes.' },
  { title: '2. Description of Service', text: 'Keepstar provides an AI-powered dynamic website engine that replaces static pages with personalized experiences. The Service includes website generation, AI-driven content personalization, analytics, and related tools. We may modify, suspend, or discontinue any aspect of the Service at any time without prior notice.' },
  { title: '3. User Accounts', text: 'You must provide accurate, complete, and current information when creating an account. You are responsible for maintaining the confidentiality of your account credentials and for all activities that occur under your account. You must notify us immediately of any unauthorized use of your account or any other breach of security.' },
  { title: '4. Intellectual Property', text: 'All content, features, and functionality of the Service — including but not limited to text, graphics, logos, icons, software, and AI-generated outputs — are owned by Keepstar or its licensors and are protected by international copyright, trademark, and other intellectual property laws. You may not reproduce, distribute, or create derivative works without our express written permission.' },
  { title: '5. Prohibited Uses', text: 'You agree not to use the Service for any unlawful purpose, to violate any laws or regulations, to infringe on the rights of others, to submit false or misleading information, to upload viruses or malicious code, to attempt to gain unauthorized access to our systems, or to interfere with the proper functioning of the Service.' },
  { title: '6. Limitation of Liability', text: 'To the maximum extent permitted by law, Keepstar shall not be liable for any indirect, incidental, special, consequential, or punitive damages, including but not limited to loss of profits, data, or business opportunities, arising out of or in connection with your use of the Service, whether based on warranty, contract, tort, or any other legal theory.' },
  { title: '7. Termination', text: 'We may terminate or suspend your access to the Service immediately, without prior notice, for any reason, including breach of these Terms. Upon termination, your right to use the Service will cease immediately. All provisions of the Terms which by their nature should survive termination shall survive.' },
  { title: '8. Contact Information', text: 'If you have any questions about these Terms of Use, please contact us at legal@keepstar.ai.' },
];

export default function TermsOfUse() {
  useEffect(() => { window.scrollTo(0, 0); }, []);

  return (
    <div className="app-container">
      <Header />
      <section className="page-hero">
        {confetti.map((s, i) => (
          <div key={i} className={`floating-shape ${s.type === 'ellipse' ? 'shape-ellipse' : 'shape-rect'} ${s.color} ${s.delay}`}
            style={{ top: s.top, left: s.left, width: s.w, height: s.h || s.w, opacity: 0.6, transform: s.rot ? `rotate(${s.rot})` : undefined }} />
        ))}
        <h1>Terms of Use</h1>
        <p className="page-date">Last updated: March 26, 2026</p>
      </section>
      <div className="page-body">
        {sections.map((s, i) => (
          <section key={i}>
            <h2>{s.title}</h2>
            <p>{s.text}</p>
          </section>
        ))}
      </div>
      <Footer />
    </div>
  );
}
