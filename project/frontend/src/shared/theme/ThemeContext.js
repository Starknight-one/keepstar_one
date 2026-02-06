import { createContext } from 'react';
import { DEFAULT_THEME } from './themeModel';

export const ThemeContext = createContext({
  theme: DEFAULT_THEME,
  setTheme: () => {},
});
