import { AtomType } from './atomModel';

// Renders any atom based on its type
export function AtomRenderer({ atom }) {
  switch (atom.type) {
    case AtomType.TEXT:
      return <span className="atom-text">{atom.value}</span>;

    case AtomType.PRICE:
      return (
        <span className="atom-price">
          {atom.meta?.currency || '$'}{atom.value}
        </span>
      );

    case AtomType.IMAGE:
      return (
        <img
          className="atom-image"
          src={atom.value}
          alt={atom.meta?.label || ''}
        />
      );

    case AtomType.RATING:
      return (
        <span className="atom-rating">
          {'★'.repeat(Math.round(atom.value))}
          {'☆'.repeat(5 - Math.round(atom.value))}
        </span>
      );

    case AtomType.BUTTON:
      return (
        <button className="atom-button" data-action={atom.meta?.action}>
          {atom.value}
        </button>
      );

    case AtomType.BADGE:
      return <span className="atom-badge">{atom.value}</span>;

    default:
      return <span>{String(atom.value)}</span>;
  }
}
