import { useEffect, useState } from 'react'
import MessageList from './MessageList'
import FilterPanel from './FilterPanel'

// ChatPanel — narrow 360px right column. Message history on top, a
// collapsible data-derived filter panel (shown when the current result set
// has usable facets), and the chat input at the bottom. On send: invoke the
// parent's `onSend` (which calls the pipeline endpoint) and append the user
// message immediately.
//
// inputRef is an optional ref the parent can pass to set the input value
// programmatically — used by the search action handler to pre-fill the chat
// with a suggested query.
export default function ChatPanel({ messages, onSend, isLoading, inputRef, facets, searchEpoch, onFilter }) {
  const [draft, setDraft] = useState('')

  // Expose a stable handle to the parent so dispatchAction (search /
  // open_category kinds) can fill the input.
  useEffect(() => {
    if (!inputRef) return
    inputRef.current = { setValue: (v) => setDraft(v) }
    return () => {
      inputRef.current = null
    }
  }, [inputRef])

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
      <FilterPanel facets={facets} searchEpoch={searchEpoch} onChange={onFilter} />
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
