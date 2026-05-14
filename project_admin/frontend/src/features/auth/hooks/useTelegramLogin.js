import { useEffect, useRef, useCallback, useState } from 'react'

// useTelegramLogin loads telegram-login.js once and returns a trigger function
// that opens the Telegram auth popup. The popup sends the signed payload back
// via postMessage; the Telegram SDK calls our callback with the user object.
// We then POST the payload to the backend's existing HMAC-verification endpoint.
export function useTelegramLogin({ botId, onAuth, enabled }) {
  const [ready, setReady] = useState(false)
  const onAuthRef = useRef(onAuth)
  onAuthRef.current = onAuth

  useEffect(() => {
    if (!enabled || !botId) return
    if (window.Telegram?.Login) { setReady(true); return }

    const script = document.createElement('script')
    script.src = 'https://telegram.org/js/telegram-login.js'
    script.async = true
    script.onload = () => setReady(true)
    document.body.appendChild(script)
    return () => {
      try { document.body.removeChild(script) } catch (_) {}
    }
  }, [enabled, botId])

  const trigger = useCallback(() => {
    if (!ready || !window.Telegram?.Login?.auth) return
    // Telegram's widget JS expects the NUMERIC bot id under `bot_id` (it's
    // mapped to OAuth's `client_id` internally). Passing the username here
    // throws "options.client_id is required" and the popup never opens.
    window.Telegram.Login.auth(
      { bot_id: botId, origin: window.location.origin, request_access: 'write' },
      (user) => { if (user) onAuthRef.current?.(user) },
    )
  }, [ready, botId])

  return { trigger, ready }
}
