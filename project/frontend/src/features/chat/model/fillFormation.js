/**
 * fillFormation — fills a template formation with entity data on the client side.
 * Mirrors Go field getters: productFieldGetter / serviceFieldGetter.
 *
 * @param {Object} template  - FormationWithData with 1 widget, atoms have fieldName + value=null
 * @param {Object} entity    - Raw entity object (product or service)
 * @param {string} entityType - "product" or "service"
 * @returns {Object|null} - Filled FormationWithData ready to render, or null if template invalid
 */
export function fillFormation(template, entity, entityType) {
  const sourceAtoms = template?.widgets?.[0]?.atomsV2 || template?.widgets?.[0]?.atoms;
  if (!sourceAtoms || !entity) return null;

  const templateWidget = template.widgets[0];
  const isV2 = Boolean(templateWidget.atomsV2 || templateWidget.layout);
  const atoms = [];

  for (const atom of sourceAtoms) {
    const value = atom.fieldName ? getField(entity, atom.fieldName, entityType) : atom.value;

    if (isV2) {
      // V2: always push (layout tree references atoms by index — skipping breaks indices)
      const filled = { ...atom };
      if (value != null) filled.value = value;
      if (atom.meta) {
        const meta = { ...atom.meta };
        if (meta.currency === '__ENTITY_CURRENCY__') meta.currency = entity.currency || '$';
        filled.meta = meta;
      }
      atoms.push(filled);
    } else {
      // V1: skip atoms with null values (no layout tree dependency)
      if (value == null) continue;
      const filled = {
        type: atom.type, subtype: atom.subtype, display: atom.display,
        value, slot: atom.slot, fieldName: atom.fieldName,
      };
      if (atom.meta) {
        const meta = { ...atom.meta };
        if (meta.currency === '__ENTITY_CURRENCY__') meta.currency = entity.currency || '$';
        filled.meta = meta;
      }
      atoms.push(filled);
    }
  }

  const widgetId = `${entityType}-${entity.id}-${Date.now().toString(36)}`;

  const widget = {
    id: widgetId,
    size: templateWidget.size,
    priority: 0,
    entityRef: { type: entityType, id: entity.id },
  };

  if (isV2) {
    widget.atomsV2 = atoms;
    if (templateWidget.layout) widget.layout = templateWidget.layout;
    if (templateWidget.actions) widget.actions = templateWidget.actions;
  } else {
    widget.template = templateWidget.template;
    widget.atoms = atoms;
  }

  return {
    mode: template.mode,
    grid: template.grid || null,
    widgets: [widget],
  };
}

/**
 * getField — mirrors Go productFieldGetter / serviceFieldGetter.
 * Returns null for empty/zero/missing values (same skip logic as backend).
 */
function getField(entity, fieldName) {
  if (!fieldName) return null;

  switch (fieldName) {
    case 'id':
      return entity.id || null;
    case 'name':
      return entity.name || null;
    case 'description':
      return entity.description || null;
    case 'price':
      return entity.price != null ? entity.price : null;
    case 'images':
      return entity.images?.length > 0 ? entity.images : null;
    case 'rating':
      return entity.rating ? entity.rating : null;
    case 'brand':
      return entity.brand || null;
    case 'category':
      return entity.category || null;
    case 'stockQuantity':
      return entity.stockQuantity ? entity.stockQuantity : null;
    case 'tags':
      return entity.tags?.length > 0 ? entity.tags : null;
    case 'attributes':
      return entity.attributes && Object.keys(entity.attributes).length > 0
        ? entity.attributes
        : null;
    case 'duration':
      return entity.duration || null;
    case 'provider':
      return entity.provider || null;
    case 'availability':
      return entity.availability || null;
    default:
      return entity[fieldName] ?? null;
  }
}
