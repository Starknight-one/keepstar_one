# Message

Сообщения чата.

## Файлы

- `messageModel.js` — MessageRole enum (USER, ASSISTANT)
- `MessageBubble.jsx` — Рендер сообщения (без CSS файла)

## MessageRole

| Role | Описание |
|------|----------|
| user | Сообщение пользователя |
| assistant | Ответ AI-ассистента |

## MessageBubble Props

- `message` — объект сообщения
- `hideFormation` — скрыть виджеты/formation в сообщении

## Особенности

- Legacy widgets: рендерит виджеты когда widgets есть, но formation нет
- New formation: рендерит FormationRenderer когда formation есть
