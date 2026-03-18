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

  // GroupWrapper: collapse → expandable section
  if (node.groupWrapper === 'collapse') {
    return <CollapseGroup node={node} atoms={atoms} />;
  }

  // GroupWrapper: carousel → horizontal scroll
  if (node.groupWrapper === 'carousel') {
    return <CarouselGroup node={node} atoms={atoms} />;
  }

  const style = buildNodeStyle(node);
  const className = `layout-${node.type || 'column'}`;

  return (
    <div className={className} style={style}>
      {node.children.map((child, i) => (
        <LayoutChild key={i} child={child} atoms={atoms} />
      ))}
    </div>
  );
}

function LayoutChild({ child, atoms }) {
  // AtomIndex reference
  if (child.atomIndex != null) {
    const atom = atoms[child.atomIndex];
    if (!atom) return null;
    return <AtomV2Renderer atom={atom} />;
  }

  // Nested node
  if (child.node) {
    return <LayoutTreeRenderer node={child.node} atoms={atoms} />;
  }

  return null;
}

function CollapseGroup({ node, atoms }) {
  const [expanded, setExpanded] = useState(false);
  const style = buildNodeStyle(node);

  return (
    <div className="layout-collapse">
      {expanded && (
        <div className={`layout-${node.type || 'flow'}`} style={style}>
          {node.children.map((child, i) => (
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

function CarouselGroup({ node, atoms }) {
  const style = {
    ...buildNodeStyle(node),
    overflowX: 'auto',
    flexWrap: 'nowrap',
  };

  return (
    <div className="layout-carousel" style={style}>
      {node.children.map((child, i) => (
        <LayoutChild key={i} child={child} atoms={atoms} />
      ))}
    </div>
  );
}

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

  return style;
}
