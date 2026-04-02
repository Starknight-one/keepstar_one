// Shared utility functions for all template components

// Group atoms by their slot field
export function groupAtomsBySlot(atoms) {
  const slots = {};
  for (const atom of atoms) {
    const slot = atom.slot || 'primary'; // Default to primary if no slot
    if (!slots[slot]) {
      slots[slot] = [];
    }
    slots[slot].push(atom);
  }
  return slots;
}

// Normalize image value to array
export function normalizeImages(value) {
  if (Array.isArray(value)) return value;
  if (typeof value === 'string') return [value];
  return [];
}
