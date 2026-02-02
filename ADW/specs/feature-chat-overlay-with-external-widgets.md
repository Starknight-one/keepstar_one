# Feature: Chat Overlay with External Widget Rendering

## Feature Description
При открытии чата весь экран затемняется (overlay/backdrop), в то время как чат остаётся светлым и визуально "поднимается" на уровень выше. Виджеты (Formation), которые приходят после второго этапа pipeline, рендерятся не внутри чата, а снаружи - в свободном пространстве слева от чата, центрируясь по доступной области.

## Objective
- Создать immersive UX при взаимодействии с чатом
- Разделить текстовый чат и визуальный контент (виджеты)
- Использовать экранное пространство эффективно: чат справа, виджеты в центре оставшегося места

## Expertise Context
Expertise used:
- **frontend-features**: Chat feature structure (ChatPanel, useChatSubmit, useChatMessages), overlay feature (useOverlayState, FullscreenOverlay)
- **frontend-entities**: Formation/Widget rendering pattern, MessageBubble structure
- **frontend-shared**: sendPipelineQuery API response format with formation

## Relevant Files

### Existing Files
- `project/frontend/src/App.jsx` - корневой компонент, управляет isChatOpen state
- `project/frontend/src/App.css` - глобальные стили и стили кнопки чата
- `project/frontend/src/features/chat/ChatPanel.jsx` - основной компонент чата
- `project/frontend/src/features/chat/ChatPanel.css` - стили чата
- `project/frontend/src/features/chat/useChatSubmit.js` - хук отправки, получает formation из pipeline
- `project/frontend/src/features/chat/useChatMessages.js` - state сообщений
- `project/frontend/src/entities/message/MessageBubble.jsx` - рендерит formation внутри сообщения
- `project/frontend/src/entities/formation/FormationRenderer.jsx` - рендерер виджетов
- `project/frontend/src/features/overlay/useOverlayState.js` - хук для overlay (уже есть)
- `project/frontend/src/features/overlay/FullscreenOverlay.jsx` - базовый overlay (stub)

### New Files (if needed)
- `project/frontend/src/features/overlay/Overlay.css` - стили для backdrop и layout

## Step by Step Tasks
IMPORTANT: Execute strictly in order.

### 1. Добавить стили для Overlay
Создать `project/frontend/src/features/overlay/Overlay.css`:
- `.chat-backdrop` - полноэкранный затемняющий слой (position: fixed, inset: 0, background: rgba(0,0,0,0.5), z-index: 999)
- `.chat-overlay-layout` - flex container для layout (чат справа, виджеты в центре слева)
- `.widget-display-area` - область для виджетов (flex: 1, center content)
- `.chat-area` - обёртка для чата (fixed position right)

### 2. Обновить App.jsx - добавить state для formation
- Добавить state: `const [activeFormation, setActiveFormation] = useState(null)`
- Передать `setActiveFormation` в ChatPanel как prop
- При `isChatOpen === true`:
  - Рендерить backdrop с затемнением
  - Рендерить ChatPanel в правой части
  - Рендерить FormationRenderer в левой/центральной части если есть activeFormation

### 3. Обновить App.css - стили для overlay layout
Добавить стили:
```css
.chat-backdrop {
  position: fixed;
  inset: 0;
  background: rgba(0, 0, 0, 0.5);
  z-index: 998;
}

.chat-overlay-layout {
  position: fixed;
  inset: 0;
  display: flex;
  z-index: 999;
  pointer-events: none;
}

.widget-display-area {
  flex: 1;
  display: flex;
  align-items: center;
  justify-content: center;
  padding: 40px;
  pointer-events: auto;
}

.chat-area {
  pointer-events: auto;
}
```

### 4. Обновить ChatPanel - передача formation наверх
- Добавить prop `onFormationReceived`
- В `useChatSubmit`: при получении response.formation вызвать `onFormationReceived(formation)`
- Убрать рендеринг FormationRenderer из MessageBubble для assistant messages (оставить только текст)

### 5. Обновить ChatPanel.css
- Убрать `position: fixed` из `.chat-container` (позиционирование будет управляться из App)
- Добавить класс для elevated состояния: `box-shadow: 0 8px 40px rgba(0, 0, 0, 0.3)`

### 6. Обновить MessageBubble - условный рендеринг formation
- Добавить prop `hideFormation?: boolean`
- Если `hideFormation === true`, не рендерить FormationRenderer
- ChatHistory должен пробрасывать этот prop

### 7. Обновить ChatHistory - проброс props
- Добавить prop `hideFormation`
- Передать в MessageBubble

### 8. Интеграция в App.jsx - финальная сборка
```jsx
{isChatOpen && (
  <>
    <div className="chat-backdrop" onClick={() => setIsChatOpen(false)} />
    <div className="chat-overlay-layout">
      <div className="widget-display-area">
        {activeFormation && <FormationRenderer formation={activeFormation} />}
      </div>
      <div className="chat-area">
        <ChatPanel
          onClose={() => setIsChatOpen(false)}
          onFormationReceived={setActiveFormation}
          hideFormation={true}
        />
      </div>
    </div>
  </>
)}
```

### 9. Обновить z-index для кнопки чата
- `.chat-toggle-btn` должна быть поверх backdrop (z-index: 1001 уже есть - OK)

### 10. Добавить анимации (опционально, улучшение UX)
- Fade-in для backdrop
- Slide-in для чата
- Fade-in для виджетов

### 11. Validation
- Запустить `npm run build` в project/frontend
- Запустить `npm run lint` в project/frontend
- Проверить визуально: открыть чат, отправить сообщение, убедиться что виджеты рендерятся снаружи

## Validation Commands
```bash
cd project/frontend && npm run build
cd project/frontend && npm run lint
```

## Acceptance Criteria
- [ ] При открытии чата экран затемняется (backdrop overlay)
- [ ] Чат визуально "поднят" над backdrop (elevated shadow)
- [ ] Чат позиционируется справа
- [ ] Виджеты (Formation) после pipeline рендерятся в центре оставшегося пространства слева от чата
- [ ] Клик по backdrop закрывает чат
- [ ] Кнопка чата остаётся кликабельной поверх backdrop
- [ ] Frontend собирается без ошибок
- [ ] Lint проходит без ошибок

## Notes
- Уже существует `useOverlayState` hook и `FullscreenOverlay` component (stub), но они не подходят напрямую - нужен кастомный layout с разделением на зоны
- Formation приходит в response.formation из `sendPipelineQuery`
- Сообщения ассистента сейчас хранят formation внутри message object - нужно "поднять" последний formation на уровень App
- Нужно решить: показывать ли старые formations или только последний. Рекомендация: только последний активный formation

## Architecture Diagram
```
┌──────────────────────────────────────────────────────────────┐
│                      BACKDROP (dimmed)                       │
│  ┌────────────────────────────────┐ ┌─────────────────────┐  │
│  │                                │ │                     │  │
│  │     WIDGET DISPLAY AREA        │ │     CHAT PANEL      │  │
│  │     (FormationRenderer)        │ │     (elevated)      │  │
│  │                                │ │                     │  │
│  │         [centered]             │ │    Messages...      │  │
│  │                                │ │    Input...         │  │
│  │                                │ │                     │  │
│  └────────────────────────────────┘ └─────────────────────┘  │
│                                                      [x]     │
└──────────────────────────────────────────────────────────────┘
                                                    ↑
                                              Toggle Button
```
