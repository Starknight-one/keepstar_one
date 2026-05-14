import { useEffect, useRef, useCallback, useState } from 'react'

// useTelegramLogin loads telegram-login.js once and returns a trigger function
// that opens the Telegram auth popup. The popup sends the signed payload back
// via postMessage; the Telegram SDK calls our callback with the user object.
// We then POST the payload to the backend's existing HMAC-verification endpoint.
export function useTelegramLogin({ botUsername, onAuth, enabled }) {
  const [ready, setReady] = useState(false)
  const onAuthRef = useRef(onAuth)
  onAuthRef.current = onAuth

  useEffect(() => {
    if (!enabled || !botUsername) return
    if (window.Telegram?.Login) { setReady(true); return }

    const script = document.createElement('script')
    script.src = 'https://telegram.org/js/telegram-login.js'
    script.async = true
    script.onload = () => setReady(true)
    document.body.appendChild(script)
    return () => {
      try { document.body.removeChild(script) } catch (_) {}
    }
  }, [enabled, botUsername])

  const trigger = useCallback(() => {
    if (!ready || !window.Telegram?.Login?.auth) return
    window.Telegram.Login.auth(
      { bot_id: botUsername, origin: window.location.origin, request_access: 'write' },
      (user) => { if (user) onAuthRef.current?.(user) },
    )
  }, [ready, botUsername])

  return { trigger, ready }
}
