// Widget types
export const WidgetType = {
  PRODUCT_CARD: 'product_card',
  PRODUCT_LIST: 'product_list',
  COMPARISON_TABLE: 'comparison_table',
  IMAGE_CAROUSEL: 'image_carousel',
  TEXT_BLOCK: 'text_block',
  QUICK_REPLIES: 'quick_replies',
};

// Formation types (layout)
export const FormationType = {
  GRID: 'grid',
  LIST: 'list',
  CAROUSEL: 'carousel',
  SINGLE: 'single',
};

// Widget structure
// {
//   id: string,
//   type: WidgetType,
//   atoms: Atom[],
//   children: Widget[],
//   meta: { title, subtitle, clickable, action }
// }
