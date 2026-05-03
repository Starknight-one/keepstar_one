import { useState } from 'react'
import MessageList from './MessageList'

// ChatPanel — narrow 360px right column. Input at the bottom, history
// list above. On send: invoke the parent's `onSend` (which calls the
// pipeline endpoint) and append the user message immediately.
//
// Loading state disables the input + button and adds a placeholder bot
// message. Error caught by parent and surfaced as an error-style
// message.

export default function ChatPanel({ messages, onSend, isLoading }) {
  const [draft, setDraft] = useState('')

  const submit = (e) => {
    e?.preventDefault()
    const value = draft.trim()
    if (!value || isLoading) return
    setDraft('')
    onSend(value)
  }

  return (
    <div className="kw-chat">
      <MessageList messages={messages} />
      <form className="kw-chat-input" onSubmit={submit}>
        <input
          type="text"
          placeholder={isLoading ? 'Loading…' : 'Ask anything…'}
          value={draft}
          onChange={(e) => setDraft(e.target.value)}
          disabled={isLoading}
        />
        <button type="submit" disabled={isLoading || !draft.trim()}>
          Send
        </button>
      </form>
    </div>
  )
}
