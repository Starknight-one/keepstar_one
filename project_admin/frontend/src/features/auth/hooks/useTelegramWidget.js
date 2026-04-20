import { useEffect } from 'react'

// useTelegramWidget injects Telegram's login widget script into the element
// referenced by `containerRef`. The widget calls `window[callbackName](user)`
// with the payload; we bridge that into React by assigning a global handler
// bound to the `onAuth` callback from the caller.
//
// Cleanup removes the script tag + the global handler so navigating away
// doesn't leak handlers across renders.
export function useTelegramWidget({ containerRef, botUsername, onAuth, enabled = true, size = 'large' }) {
  useEffect(() => {
    if (!enabled || !botUsername || !containerRef.current) return

    const callbackName = `__keepstarTelegramAuth_${Math.random().toString(36).slice(2)}`
    window[callbackName] = (user) => {
      try { onAuth?.(user) } catch (_) {}
    }

    const script = document.createElement('script')
    script.async = true
    script.src = 'https://telegram.org/js/telegram-widget.js?22'
    script.setAttribute('data-telegram-login', botUsername)
    script.setAttribute('data-size', size)
    script.setAttribute('data-onauth', `${callbackName}(user)`)
    script.setAttribute('data-request-access', 'write')
    script.setAttribute('data-userpic', 'false')

    const el = containerRef.current
    el.innerHTML = ''
    el.appendChild(script)

    return () => {
      try { delete window[callbackName] } catch (_) { window[callbackName] = undefined }
      if (el) el.innerHTML = ''
    }
  }, [enabled, botUsername, size, containerRef, onAuth])
}
