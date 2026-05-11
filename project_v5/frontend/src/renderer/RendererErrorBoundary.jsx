import React from 'react'

// RendererErrorBoundary catches render errors thrown by any node component
// in the scene-graph tree and logs them without crashing the widget.
// On error: renders null (widget area blank) until the next valid document
// arrives, at which point the boundary resets automatically via resetKey.
export default class RendererErrorBoundary extends React.Component {
  state = { hasError: false, resetKey: null }

  static getDerivedStateFromError() {
    return { hasError: true }
  }

  componentDidCatch(err, info) {
    console.error(
      '[v5-renderer] node render error — showing blank until next valid document',
      err.message,
      info?.componentStack?.slice(0, 300),
    )
  }

  // Auto-reset when parent provides a new document object (resetKey changes).
  static getDerivedStateFromProps(nextProps, state) {
    if (state.hasError && nextProps.resetKey !== state.resetKey) {
      return { hasError: false, resetKey: nextProps.resetKey }
    }
    if (!state.hasError) {
      return { resetKey: nextProps.resetKey }
    }
    return null
  }

  render() {
    if (this.state.hasError) return null
    return this.props.children
  }
}
