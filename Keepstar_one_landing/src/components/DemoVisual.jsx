import React from 'react';
import './DemoVisual.css';

export default function DemoVisual() {
  return (
    <section className="demo-section">
      <div className="section-header text-center">
        <div className="badge">Инновация</div>
        <h2>Больше, чем текст: <span className="text-gradient">Интерфейс</span></h2>
        <p>Оцените разницу. Зачем заставлять клиента переходить по текстовым ссылкам, если можно дать полноценную витрину?</p>
      </div>
      
      <div className="demo-container">
        
        {/* Mock Chat Window */}
        <div className="mock-chat glass-panel">
          <div className="chat-header">
            <div className="dots"><span/><span/><span/></div>
            <div className="chat-title">Keepstar Assistant</div>
          </div>
          
          <div className="chat-body">
            {/* User message */}
            <div className="message user-msg animate-slide-up">
              Покажи стильные красные кроссовки Найк
            </div>
            
            {/* Bot message - Text + Widget */}
            <div className="message bot-msg widget-bundle animate-slide-up delay-100">
              <div className="bot-text">Я нашел отличные варианты Nike для вас:</div>
              
              <div className="widget-mock">
                <div className="widget-header">
                  <span>👟 AI Collection</span>
                  <span className="widget-meta">2 найдено</span>
                </div>
                <div className="widget-cards">
                  <div className="w-card hover-glow">
                    <div className="w-img img-red1"></div>
                    <div className="w-info">
                      <div className="w-title">Air Max Plus</div>
                      <div className="w-rating">★★★★★ <span className="w-reviews">(124)</span></div>
                      <div className="w-price-row">
                        <span className="w-price">18 500 ₽</span>
                        <button className="w-btn">+</button>
                      </div>
                    </div>
                  </div>
                  
                  <div className="w-card hover-glow">
                    <div className="w-img img-red2"></div>
                    <div className="w-info">
                      <div className="w-title">Air Jordan 1</div>
                      <div className="w-rating">★★★★☆ <span className="w-reviews">(89)</span></div>
                      <div className="w-price-row">
                        <span className="w-price">21 990 ₽</span>
                        <button className="w-btn">+</button>
                      </div>
                    </div>
                  </div>
                </div>
              </div>
            </div>
            
            {/* User message */}
            <div className="message user-msg animate-slide-up delay-300">
              А теперь сделай из них сравнительную таблицу
            </div>

            {/* Typing indicator */}
            <div className="message bot-msg typing-msg animate-slide-up delay-300">
              <div className="typing-dot"></div>
              <div className="typing-dot"></div>
              <div className="typing-dot"></div>
            </div>

          </div>
        </div>

      </div>
    </section>
  );
}
