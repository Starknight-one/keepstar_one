import { useState, useEffect } from 'react';
import { ThemeContext } from './ThemeContext';
import { ThemeType, DEFAULT_THEME } from './themeModel';

export function ThemeProvider({ children, defaultTheme = DEFAULT_THEME }) {
  const [theme, setTheme] = useState(() => {
    // Check localStorage for saved theme preference
    if (typeof window !== 'undefined') {
      const saved = localStorage.getItem('theme');
      if (saved && Object.values(ThemeType).includes(saved)) {
        return saved;
      }
    }
    return defaultTheme;
  });

  useEffect(() => {
    // Save theme preference to localStorage
    localStorage.setItem('theme', theme);
  }, [theme]);

  return (
    <ThemeContext.Provider value={{ theme, setTheme }}>
      <div className={`theme-${theme}`} data-theme={theme}>
        {children}
      </div>
    </ThemeContext.Provider>
  );
}
