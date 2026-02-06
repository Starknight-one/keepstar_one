// Available themes
export const ThemeType = {
  MARKETPLACE: 'marketplace',
  DARK: 'dark',
  LIGHT: 'light',
};

// Default theme
export const DEFAULT_THEME = ThemeType.MARKETPLACE;

// Theme metadata
export const ThemeMeta = {
  [ThemeType.MARKETPLACE]: {
    name: 'Marketplace',
    description: 'Default marketplace theme with blue accents',
  },
  [ThemeType.DARK]: {
    name: 'Dark',
    description: 'Dark mode theme',
  },
  [ThemeType.LIGHT]: {
    name: 'Light',
    description: 'Light mode theme',
  },
};
