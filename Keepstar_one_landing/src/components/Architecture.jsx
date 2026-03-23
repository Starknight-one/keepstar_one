import React from 'react';
import './Architecture.css';

export default function Architecture() {
  return (
    <section className="arch-section">
      <div className="section-header text-center">
        <div className="badge">Under the hood</div>
        <h2>Магия под капотом</h2>
        <p>Архитектура двух агентов: как обычный текст превращается в UI.</p>
      </div>

      <div className="arch-container">
        <div className="arch-step glass-panel hover-glow-blue">
          <div className="step-number">1</div>
          <div className="step-content">
            <h3>Agent 1: Анализ (NLU)</h3>
            <p>Распознает интент пользователя, осуществляет гибридный поиск по каталогу, фильтрует по стейту и обращается к истории сессии.</p>
            <div className="step-tech">LLM + pgvector + RRF</div>
          </div>
        </div>

        <div className="arch-connector"></div>

        <div className="arch-step glass-panel hover-glow-orange">
          <div className="step-number">2</div>
          <div className="step-content">
            <h3>Agent 2: Сборка (Visual Assembly)</h3>
            <p>Вызывает engine для трансформации сырых данных в атомы и виджеты. Применяет 30+ visual constraints и собирает formation JSON.</p>
            <div className="step-tech">Constraints Engine + Pipeline</div>
          </div>
        </div>

        <div className="arch-connector"></div>

        <div className="arch-step glass-panel hover-glow-blue">
          <div className="step-number">3</div>
          <div className="step-content">
            <h3>Рендер (React Frontend)</h3>
            <p>Динамически располагает виджеты (grid, list, stack) прямо в Shadow DOM поверх сайта клиента без ручного кода.</p>
            <div className="step-tech">Shadow DOM + Feature Sliced</div>
          </div>
        </div>
      </div>
    </section>
  );
}
