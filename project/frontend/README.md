# Keepstar Frontend

React chat widget for Keepstar.

## Stack

- React 19
- Vite 7
- ESLint

## Setup

```bash
# Install dependencies
npm install

# Run dev server
npm run dev

# Build for production
npm run build

# Lint
npm run lint
```

## Scripts

| Command | Description |
|---------|-------------|
| `npm run dev` | Start dev server (http://localhost:5173) |
| `npm run build` | Production build |
| `npm run preview` | Preview production build |
| `npm run lint` | Run ESLint |

## Project Structure

```
frontend/
├── src/
│   ├── components/
│   │   ├── Chat.jsx      # Chat widget component
│   │   └── Chat.css
│   ├── App.jsx           # Main app with chat toggle
│   ├── App.css
│   ├── main.jsx          # Entry point
│   └── index.css
├── public/
├── index.html
├── vite.config.js
├── eslint.config.js
└── package.json
```

## Configuration

Backend API URL is expected at `http://localhost:8080`. Update in Chat.jsx if needed.
