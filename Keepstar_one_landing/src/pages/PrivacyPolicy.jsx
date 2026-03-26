import React, { useEffect } from 'react';
import Header from '../components/Header';
import Footer from '../components/Footer';
import './StaticPage.css';

const confetti = [
  { type: 'ellipse', color: 'bg-blue', w: 5, top: '17%', left: '7%', delay: '' },
  { type: 'ellipse', color: 'bg-red', w: 4, top: '30%', left: '21%', delay: 'delay-100' },
  { type: 'rect', color: 'bg-yellow', w: 8, h: 3, top: '10%', left: '36%', rot: '30deg', delay: 'delay-200' },
  { type: 'ellipse', color: 'bg-green', w: 6, top: '24%', left: '51%', delay: '' },
  { type: 'ellipse', color: 'bg-blue', w: 3, top: '34%', left: '64%', delay: 'delay-100' },
  { type: 'rect', color: 'bg-red', w: 7, h: 3, top: '14%', left: '77%', rot: '-20deg', delay: 'delay-200' },
  { type: 'ellipse', color: 'bg-yellow', w: 5, top: '28%', left: '91%', delay: 'delay-300' },
  { type: 'ellipse', color: 'bg-green', w: 4, top: '90%', left: '14%', delay: '' },
  { type: 'rect', color: 'bg-blue', w: 6, h: 3, top: '83%', left: '46%', rot: '45deg', delay: 'delay-100' },
  { type: 'ellipse', color: 'bg-red', w: 5, top: '79%', left: '82%', delay: 'delay-200' },
  { type: 'rect', color: 'bg-yellow', w: 9, h: 3, top: '96%', left: '32%', rot: '-35deg', delay: 'delay-300' },
  { type: 'ellipse', color: 'bg-green', w: 3, top: '93%', left: '60%', delay: '' },
];

const sections = [
  { title: '1. Information We Collect', text: 'We collect information you provide directly to us, such as your name, email address, and payment information when you create an account or use our services. We also automatically collect certain information when you visit our website, including your IP address, browser type, operating system, referring URLs, and information about how you interact with our Service.' },
  { title: '2. How We Use Your Information', text: 'We use the information we collect to provide, maintain, and improve our services, to process transactions and send related information, to send you technical notices and support messages, to respond to your comments and questions, and to develop new products and services. We may also use aggregated and anonymized data for analytics and machine learning model improvement.' },
  { title: '3. Data Sharing and Disclosure', text: 'We do not sell your personal information. We may share your information with third-party service providers who perform services on our behalf, such as payment processing, data analysis, email delivery, and hosting services. We may also disclose your information if required by law, regulation, or legal process, or if we believe disclosure is necessary to protect the rights, property, or safety of Keepstar, our users, or others.' },
  { title: '4. Cookies and Tracking Technologies', text: 'We use cookies, web beacons, and similar tracking technologies to collect information about your browsing activities. You can set your browser to refuse all or some browser cookies, or to alert you when cookies are being sent. If you disable or refuse cookies, some parts of the Service may become inaccessible or not function properly.' },
  { title: '5. Data Security', text: 'We implement appropriate technical and organizational measures to protect your personal information against unauthorized access, alteration, disclosure, or destruction. However, no method of transmission over the Internet or method of electronic storage is 100% secure, and we cannot guarantee absolute security of your data.' },
  { title: '6. Your Rights', text: 'Depending on your jurisdiction, you may have the right to access, correct, or delete your personal information, to object to or restrict certain processing, and to data portability. To exercise these rights, please contact us at privacy@keepstar.ai. We will respond to your request within 30 days.' },
  { title: '7. Changes to This Policy', text: 'We may update this Privacy Policy from time to time. We will notify you of any changes by posting the new Privacy Policy on this page and updating the "Last updated" date. Your continued use of the Service after any changes constitutes your acceptance of the new Privacy Policy.' },
  { title: '8. Contact Us', text: 'If you have any questions about this Privacy Policy, please contact us at privacy@keepstar.ai.' },
];

export default function PrivacyPolicy() {
  useEffect(() => { window.scrollTo(0, 0); }, []);

  return (
    <div className="app-container">
      <Header />
      <section className="page-hero">
        {confetti.map((s, i) => (
          <div key={i} className={`floating-shape ${s.type === 'ellipse' ? 'shape-ellipse' : 'shape-rect'} ${s.color} ${s.delay}`}
            style={{ top: s.top, left: s.left, width: s.w, height: s.h || s.w, opacity: 0.6, transform: s.rot ? `rotate(${s.rot})` : undefined }} />
        ))}
        <h1>Privacy Policy</h1>
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
