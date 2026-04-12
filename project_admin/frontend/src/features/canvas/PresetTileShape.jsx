import {
  BaseBoxShapeUtil,
  HTMLContainer,
  T,
} from 'tldraw'

// --- Shape definition ---

export const PRESET_TILE_TYPE = 'preset-tile'

export const presetTileMigrations = {
  currentVersion: 0,
  migrators: {},
}

export const presetTileShapeProps = {
  w: T.number,
  h: T.number,
  presetId: T.string,
  name: T.string,
  category: T.string,
  description: T.string,
  status: T.string,
  defaultReplicate: T.boolean,
  opsCount: T.number,
}

// --- Shape util ---

export class PresetTileShapeUtil extends BaseBoxShapeUtil {
  static type = PRESET_TILE_TYPE
  static props = presetTileShapeProps
  static migrations = presetTileMigrations

  getDefaultProps() {
    return {
      w: 260,
      h: 160,
      presetId: '',
      name: 'untitled',
      category: 'product',
      description: '',
      status: 'draft',
      defaultReplicate: true,
      opsCount: 0,
    }
  }

  canEdit() { return false }
  canResize() { return true }
  isAspectRatioLocked() { return false }

  component(shape) {
    const { name, category, description, status, defaultReplicate, opsCount } = shape.props
    return (
      <HTMLContainer
        id={shape.id}
        style={{
          width: '100%',
          height: '100%',
          pointerEvents: 'all',
        }}
      >
        <div style={{
          width: '100%',
          height: '100%',
          background: '#fff',
          borderRadius: '8px',
          border: '1px solid #e5e7eb',
          boxShadow: '0 1px 3px rgba(0,0,0,0.06)',
          display: 'flex',
          flexDirection: 'column',
          overflow: 'hidden',
          fontFamily: '-apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, sans-serif',
        }}>
          {/* Header */}
          <div style={{
            padding: '12px 14px 8px',
            borderBottom: '1px solid #f3f4f6',
            display: 'flex',
            alignItems: 'center',
            justifyContent: 'space-between',
            gap: '8px',
          }}>
            <span style={{
              fontSize: '13px',
              fontWeight: 600,
              color: '#111',
              overflow: 'hidden',
              textOverflow: 'ellipsis',
              whiteSpace: 'nowrap',
            }}>
              {name}
            </span>
            <span style={{
              fontSize: '10px',
              fontWeight: 500,
              padding: '2px 6px',
              borderRadius: '99px',
              flexShrink: 0,
              background: status === 'published' ? '#d1fae5' : '#fef3c7',
              color: status === 'published' ? '#065f46' : '#92400e',
            }}>
              {status}
            </span>
          </div>

          {/* Body */}
          <div style={{
            flex: 1,
            padding: '8px 14px',
            display: 'flex',
            flexDirection: 'column',
            gap: '6px',
          }}>
            <div style={{ fontSize: '11px', color: '#6b7280' }}>
              {category}{defaultReplicate ? ' \u00b7 replicate' : ''}
            </div>
            {description && (
              <div style={{
                fontSize: '12px',
                color: '#374151',
                lineHeight: '1.4',
                overflow: 'hidden',
                display: '-webkit-box',
                WebkitLineClamp: 2,
                WebkitBoxOrient: 'vertical',
              }}>
                {description}
              </div>
            )}
          </div>

          {/* Footer */}
          <div style={{
            padding: '6px 14px 10px',
            fontSize: '11px',
            color: '#9ca3af',
            borderTop: '1px solid #f3f4f6',
          }}>
            {opsCount > 0 ? `${opsCount} ops` : 'empty'}
          </div>
        </div>
      </HTMLContainer>
    )
  }

  indicator(shape) {
    return (
      <rect
        width={shape.props.w}
        height={shape.props.h}
        rx={8}
        ry={8}
      />
    )
  }
}
