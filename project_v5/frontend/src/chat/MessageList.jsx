// Message list — renders the chat history (user prompts + bot
// acknowledgments + errors). The bot doesn't speak — it renders
// widgets — but we surface a one-line "rendered N nodes" ack per turn
// so the user knows something happened.

export default function MessageList({ messages }) {
  return (
    <div className="kw-chat-history">
      {messages.map((m, i) => (
        <div
          key={i}
          className={
            'kw-msg ' +
            (m.role === 'user'
              ? 'kw-msg-user'
              : m.role === 'error'
              ? 'kw-msg-error'
              : 'kw-msg-bot')
          }
        >
          {m.text}
        </div>
      ))}
    </div>
  )
}
