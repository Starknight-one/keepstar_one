import { useState } from 'react';
import { AtomV2Renderer } from '../../atom/AtomV2Renderer';

const SPACING_TOKENS = { none: 0, xs: 2, sm: 4, md: 8, lg: 12, xl: 16, '2xl': 24 };

function resolveSpacing(token) {
  if (typeof token === 'number') return token;
  return SPACING_TOKENS[token] ?? 8;
}

/**
 * LayoutTreeRenderer — recursively renders a v2 LayoutNode tree.
 * Maps node types to CSS flex layouts:
 *   row → flex-direction: row
 *   column → flex-direction: column
 *   flow → flex-wrap: wrap (inline flow)
 *   span → full-width block
 */
export function LayoutTreeRenderer({ node, atoms }) {
  if (!node || !node.children || node.children.length === 0) return null;

  const children = node.children;

  if (children.length === 0) return null;

  // GroupWrapper: collapse → expandable section
  if (node.groupWrapper === 'collapse') {
    return <CollapseGroup node={node} atoms={atoms} children={children} />;
  }

  // GroupWrapper: carousel → horizontal scroll
  if (node.groupWrapper === 'carousel') {
    return <CarouselGroup node={node} atoms={atoms} children={children} />;
  }

  const style = buildNodeStyle(node);
  const className = `layout-${node.type || 'column'}`;

  return (
    <div className={className} style={style}>
      {children.map((child, i) => (
        <LayoutChild key={i} child={child} atoms={atoms} />
      ))}
    </div>
  );
}

function LayoutChild({ child, atoms }) {
  const childStyle = {};
  if (child.grow) childStyle.flexGrow = child.grow;
  if (child.selfAlign) childStyle.alignSelf = child.selfAlign;
  const hasStyle = Object.keys(childStyle).length > 0;

  // AtomIndex reference
  if (child.atomIndex != null) {
    const atom = atoms[child.atomIndex];
    if (!atom) return null;
    if (hasStyle) {
      return <div style={childStyle}><AtomV2Renderer atom={atom} /></div>;
    }
    return <AtomV2Renderer atom={atom} />;
  }

  // Nested node
  if (child.node) {
    if (hasStyle) {
      return <div style={childStyle}><LayoutTreeRenderer node={child.node} atoms={atoms} /></div>;
    }
    return <LayoutTreeRenderer node={child.node} atoms={atoms} />;
  }

  return null;
}

function CollapseGroup({ node, atoms, children }) {
  const [expanded, setExpanded] = useState(false);
  const style = buildNodeStyle(node);
  const items = children || node.children;

  return (
    <div className="layout-collapse">
      {expanded && (
        <div className={`layout-${node.type || 'flow'}`} style={style}>
          {items.map((child, i) => (
            <LayoutChild key={i} child={child} atoms={atoms} />
          ))}
        </div>
      )}
      <button
        className="layout-collapse-toggle"
        onClick={() => setExpanded(!expanded)}
      >
        {expanded ? 'Скрыть' : 'Показать ещё'}
      </button>
    </div>
  );
}

function CarouselGroup({ node, atoms, children }) {
  const style = {
    ...buildNodeStyle(node),
    overflowX: 'auto',
    flexWrap: 'nowrap',
  };
  const items = children || node.children;

  return (
    <div className="layout-carousel" style={style}>
      {items.map((child, i) => (
        <LayoutChild key={i} child={child} atoms={atoms} />
      ))}
    </div>
  );
}

const RADIUS_TOKENS = { none: '0', sm: '4px', md: '8px', lg: '12px', xl: '16px', full: '9999px' };
const SHADOW_TOKENS = {
  none: 'none',
  sm: '0 1px 2px rgba(0,0,0,0.05)',
  md: '0 4px 6px rgba(0,0,0,0.1)',
  lg: '0 10px 15px rgba(0,0,0,0.1)',
};
const DISTRIBUTION_MAP = {
  start: 'flex-start', center: 'center', end: 'flex-end',
  between: 'space-between', around: 'space-around', evenly: 'space-evenly',
};

function buildNodeStyle(node) {
  const style = {};

  // Flex direction based on type
  switch (node.type) {
    case 'row':
      style.display = 'flex';
      style.flexDirection = 'row';
      style.alignItems = node.align || 'center';
      break;
    case 'column':
      style.display = 'flex';
      style.flexDirection = 'column';
      break;
    case 'flow':
      style.display = 'flex';
      style.flexDirection = 'row';
      style.flexWrap = 'wrap';
      style.alignItems = node.align || 'flex-start';
      break;
    case 'span':
      style.display = 'block';
      style.width = '100%';
      break;
    default:
      style.display = 'flex';
      style.flexDirection = 'column';
  }

  // Gap
  if (node.gap) {
    style.gap = resolveSpacing(node.gap) + 'px';
  }

  // Distribution (justify-content)
  if (node.distribution) {
    style.justifyContent = DISTRIBUTION_MAP[node.distribution] || node.distribution;
  }

  // Visual container properties
  if (node.borderRadius) {
    style.borderRadius = RADIUS_TOKENS[node.borderRadius] || node.borderRadius;
    style.overflow = 'hidden';
  }
  if (node.shadow) {
    style.boxShadow = SHADOW_TOKENS[node.shadow] || node.shadow;
  }
  if (node.padding) {
    style.padding = resolveSpacing(node.padding) + 'px';
  }
  if (node.background) {
    style.background = node.background;
  }

  return style;
}
