import { useEffect } from 'react'

// useTelegramWidget injects Telegram's login widget script into the element
// referenced by `containerRef`. The widget calls `window[callbackName](user)`
// with the payload; we bridge that into React by assigning a global handler
// bound to the `onAuth` callback from the caller.
//
// The Telegram iframe has a fixed width (~220px). We use a ResizeObserver to
// scale it to fill the container so it matches sibling full-width buttons.
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

    // Scale the iframe to fill the container width once it appears.
    let ro = null
    const scaleIframe = () => {
      const iframe = el.querySelector('iframe')
      if (!iframe) return
      const containerW = el.offsetWidth
      const iframeW = iframe.offsetWidth
      if (!containerW || !iframeW) return
      const scale = containerW / iframeW
      iframe.style.transformOrigin = '0 0'
      iframe.style.transform = `scaleX(${scale})`
      el.style.height = iframe.offsetHeight + 'px'
    }

    // MutationObserver catches the iframe being injected by Telegram's script.
    const mo = new MutationObserver(() => {
      const iframe = el.querySelector('iframe')
      if (!iframe) return
      mo.disconnect()
      // Wait one frame for the iframe to get its intrinsic size.
      requestAnimationFrame(() => {
        scaleIframe()
        ro = new ResizeObserver(scaleIframe)
        ro.observe(el)
      })
    })
    mo.observe(el, { childList: true, subtree: true })

    return () => {
      mo.disconnect()
      ro?.disconnect()
      try { delete window[callbackName] } catch (_) { window[callbackName] = undefined }
      if (el) el.innerHTML = ''
    }
  }, [enabled, botUsername, size, containerRef, onAuth])
}
