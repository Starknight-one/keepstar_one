import React from 'react';
import './Features.css';

export default function Features() {
  const b2bFeatures = [
    { title: "Zero-code интеграция", desc: "Просто вставьте один <script> на ваш сайт — остальное сделает магия AI.", icon: "⚡" },
    { title: "Boost конверсии", desc: "Клиенты видят карточки товаров с кнопкой 'В корзину' прямо в чате.", icon: "📈" },
    { title: "Строгий UI / UX", desc: "Layout Engine сам подбирает дизайн под количество элементов.", icon: "🎨" },
  ];

  const b2cFeatures = [
    { title: "Диалог с контекстом", desc: "«Покажи только с 5 звёздами» — и бот отфильтрует товары на лету.", icon: "💬" },
    { title: "Сравнения товаров", desc: "Интерактивная таблица для сравнения фич прямо внутри переписки.", icon: "⚖️" },
    { title: "Shadow DOM Widget", desc: "Быстро, безопасно и без конфликтов со стилями вашего сайта.", icon: "✨" },
  ];

  return (
    <section className="features-section">
      <div className="section-header">
        <div className="badge">Преимущества</div>
        <h2>Технология, создающая <span className="text-gradient">результат</span></h2>
        <p>Идеальный баланс между скоростью внедрения и пользовательским опытом.</p>
      </div>

      <div className="features-grid">
        <div className="feature-column">
          <h3 className="column-title text-gradient">B2B (Бизнесам)</h3>
          {b2bFeatures.map((f, i) => (
            <div key={i} className="glass-panel feature-card">
              <div className="feature-icon">{f.icon}</div>
              <div>
                <h4>{f.title}</h4>
                <p>{f.desc}</p>
              </div>
            </div>
          ))}
        </div>
        <div className="feature-column">
          <h3 className="column-title text-gradient">B2C (Покупателям)</h3>
          {b2cFeatures.map((f, i) => (
            <div key={i} className="glass-panel feature-card">
              <div className="feature-icon">{f.icon}</div>
              <div>
                <h4>{f.title}</h4>
                <p>{f.desc}</p>
              </div>
            </div>
          ))}
        </div>
      </div>
    </section>
  );
}
