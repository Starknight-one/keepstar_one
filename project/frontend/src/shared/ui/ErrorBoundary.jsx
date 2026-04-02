import { Component } from 'react';

export class ErrorBoundary extends Component {
  constructor(props) {
    super(props);
    this.state = { hasError: false };
  }

  static getDerivedStateFromError() {
    return { hasError: true };
  }

  componentDidCatch(error, info) {
    console.error('[ErrorBoundary]', error, info?.componentStack);
  }

  render() {
    if (this.state.hasError) {
      return (
        <div style={{
          padding: '12px 16px',
          color: '#71717A',
          fontSize: '13px',
          textAlign: 'center'
        }}>
          {this.props.fallback || 'Something went wrong'}
        </div>
      );
    }
    return this.props.children;
  }
}
