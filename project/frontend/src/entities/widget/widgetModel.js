// Widget types (legacy, for backward compatibility)
export const WidgetType = {
  PRODUCT_CARD: 'product_card',
  PRODUCT_LIST: 'product_list',
  COMPARISON_TABLE: 'comparison_table',
  IMAGE_CAROUSEL: 'image_carousel',
  TEXT_BLOCK: 'text_block',
  QUICK_REPLIES: 'quick_replies',
};

// Widget templates (new system)
export const WidgetTemplate = {
  PRODUCT_CARD: 'ProductCard',
  SERVICE_CARD: 'ServiceCard',
};

// Formation types (layout)
export const FormationType = {
  GRID: 'grid',
  LIST: 'list',
  CAROUSEL: 'carousel',
  SINGLE: 'single',
};

// Widget sizes with constraints
export const WidgetSize = {
  TINY: 'tiny',     // 80-110px, max 2 atoms
  SMALL: 'small',   // 160-220px, max 3 atoms
  MEDIUM: 'medium', // 280-350px, max 5 atoms
  LARGE: 'large',   // 384-460px, max 10 atoms
};

// Widget structure
// {
//   id: string,
//   type: WidgetType,
//   size: WidgetSize,
//   atoms: Atom[],
//   children: Widget[],
//   meta: { title, subtitle, clickable, action }
// }
