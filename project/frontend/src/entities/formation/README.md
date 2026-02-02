# Formation

Layout виджетов — расположение группы виджетов.

## Файлы

- `formationModel.js` — Режимы layout (FormationMode)
- `FormationRenderer.jsx` — Рендерер formation
- `Formation.css` — Стили layout
- `index.js` — Экспорты

## Режимы (FormationMode)

| Mode | Описание | CSS класс |
|------|----------|-----------|
| GRID | Сетка с колонками | formation-grid cols-{n} |
| CAROUSEL | Горизонтальный скролл | formation-carousel |
| SINGLE | Один виджет | formation-single |
| LIST | Вертикальный список | formation-list |

## Использование

```jsx
import { FormationRenderer } from './FormationRenderer';

<FormationRenderer formation={{
  mode: 'grid',
  grid: { cols: 2 },
  widgets: [...]
}} />
```

## Структура formation

```js
{
  mode: 'grid' | 'carousel' | 'single' | 'list',
  grid: { rows: number, cols: number },  // для grid mode
  widgets: Widget[]
}
```
