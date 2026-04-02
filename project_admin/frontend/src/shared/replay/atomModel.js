// 6 base atom types
export const AtomType = {
  TEXT: 'text',
  NUMBER: 'number',
  IMAGE: 'image',
  ICON: 'icon',
  VIDEO: 'video',
  AUDIO: 'audio',
};

// Atom subtypes (data formats)
export const AtomSubtype = {
  // text subtypes
  STRING: 'string',
  DATE: 'date',
  DATETIME: 'datetime',
  URL: 'url',
  EMAIL: 'email',
  PHONE: 'phone',
  // number subtypes
  INT: 'int',
  FLOAT: 'float',
  CURRENCY: 'currency',
  PERCENT: 'percent',
  RATING: 'rating',
  // image subtypes
  IMAGE_URL: 'url',
  IMAGE_BASE64: 'base64',
  // icon subtypes
  ICON_NAME: 'name',
  ICON_EMOJI: 'emoji',
  ICON_SVG: 'svg',
};

// Display formats (visual presentation)
export const AtomDisplay = {
  // text displays
  H1: 'h1',
  H2: 'h2',
  H3: 'h3',
  H4: 'h4',
  BODY_LG: 'body-lg',
  BODY: 'body',
  BODY_SM: 'body-sm',
  CAPTION: 'caption',
  BADGE: 'badge',
  BADGE_SUCCESS: 'badge-success',
  BADGE_ERROR: 'badge-error',
  BADGE_WARNING: 'badge-warning',
  TAG: 'tag',
  TAG_ACTIVE: 'tag-active',
  // number displays
  PRICE: 'price',
  PRICE_LG: 'price-lg',
  PRICE_OLD: 'price-old',
  PRICE_DISCOUNT: 'price-discount',
  RATING: 'rating',
  RATING_TEXT: 'rating-text',
  RATING_COMPACT: 'rating-compact',
  PERCENT: 'percent',
  PROGRESS: 'progress',
  // image displays
  IMAGE: 'image',
  IMAGE_COVER: 'image-cover',
  AVATAR: 'avatar',
  AVATAR_SM: 'avatar-sm',
  AVATAR_LG: 'avatar-lg',
  THUMBNAIL: 'thumbnail',
  GALLERY: 'gallery',
  // icon displays
  ICON: 'icon',
  ICON_SM: 'icon-sm',
  ICON_LG: 'icon-lg',
  // interactive displays
  BUTTON_PRIMARY: 'button-primary',
  BUTTON_SECONDARY: 'button-secondary',
  BUTTON_OUTLINE: 'button-outline',
  BUTTON_GHOST: 'button-ghost',
  INPUT: 'input',
  // layout displays
  DIVIDER: 'divider',
  SPACER: 'spacer',
};

// Legacy type mapping for backward compatibility
// Maps old atom types to display values
export const LEGACY_TYPE_TO_DISPLAY = {
  price: 'price',
  badge: 'badge',
  rating: 'rating',
  button: 'button-primary',
  divider: 'divider',
  progress: 'progress',
  selector: 'tag',
};

// Atom structure (for documentation)
// {
//   type: AtomType,        // base type: text, number, image, icon, video, audio
//   subtype: AtomSubtype,  // data format: string, currency, rating, url, etc.
//   display: string,       // visual format: h1, price-lg, badge, etc.
//   value: any,
//   slot: string,          // template slot: hero, title, price, primary, etc.
//   meta: { label, unit, currency, action, link, style }
// }
