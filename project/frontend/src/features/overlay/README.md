# Overlay Feature

Fullscreen overlay для чата.

## Файлы

- `useOverlayState.js` — Хук для состояния overlay: { isOpen, open, close, toggle }
- `FullscreenOverlay.jsx` — Компонент overlay (backdrop + content)
- `Overlay.css` — Backdrop, layout, animations

## Animations

- `backdrop-fade-in` — плавное появление фона
- `chat-slide-in` — слайд чата справа
- `widget-fade-in` — появление виджетов с масштабированием
