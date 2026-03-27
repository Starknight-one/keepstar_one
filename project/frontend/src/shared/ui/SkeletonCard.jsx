import React from 'react';
import './SkeletonCard.css';

export default function SkeletonCard() {
  return (
    <div className="ui-skeleton-card">
      <div className="ui-skeleton-img" />
      <div className="ui-skeleton-body">
        <div className="ui-skeleton-line ui-skeleton-line--w100" />
        <div className="ui-skeleton-line ui-skeleton-line--w60" />
        <div className="ui-skeleton-line ui-skeleton-line--w40" />
      </div>
    </div>
  );
}
