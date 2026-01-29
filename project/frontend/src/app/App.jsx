import { useOverlayState } from '../features/overlay/useOverlayState';
import { FullscreenOverlay } from '../features/overlay/FullscreenOverlay';
import { ChatPanel } from '../features/chat/ChatPanel';
import './App.css';

export default function App() {
  const overlay = useOverlayState();

  return (
    <div className="app">
      <button className="chat-trigger" onClick={overlay.open}>
        Chat
      </button>

      <FullscreenOverlay isOpen={overlay.isOpen} onClose={overlay.close}>
        <ChatPanel />
      </FullscreenOverlay>
    </div>
  );
}
