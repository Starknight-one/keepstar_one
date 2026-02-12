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
  if (!template?.widgets?.[0]?.atoms || !entity) return null;

  const templateWidget = template.widgets[0];
  const atoms = [];

  for (const atom of templateWidget.atoms) {
    const value = getField(entity, atom.fieldName, entityType);
    if (value == null) continue;

    const filled = {
      type: atom.type,
      subtype: atom.subtype,
      display: atom.display,
      value,
      slot: atom.slot,
    };

    // Resolve currency sentinel
    if (atom.meta) {
      const meta = { ...atom.meta };
      if (meta.currency === '__ENTITY_CURRENCY__') {
        meta.currency = entity.currency || '$';
      }
      filled.meta = meta;
    }

    atoms.push(filled);
  }

  const widgetId = `${entityType}-${entity.id}-${Date.now().toString(36)}`;

  return {
    mode: template.mode,
    grid: template.grid || null,
    widgets: [{
      id: widgetId,
      template: templateWidget.template,
      size: templateWidget.size,
      priority: 0,
      atoms,
      entityRef: { type: entityType, id: entity.id },
    }],
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
