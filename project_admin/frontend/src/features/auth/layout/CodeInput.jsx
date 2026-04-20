import React, { useRef } from 'react'

export default function CodeInput({ value = '', onChange, length = 6 }) {
  const refs = useRef([])

  function setAt(idx, digit) {
    const arr = value.padEnd(length, ' ').slice(0, length).split('')
    arr[idx] = digit || ' '
    const next = arr.join('').replace(/ /g, '')
    onChange(next)
  }

  function handleChange(idx, e) {
    const raw = e.target.value.replace(/\D/g, '').slice(-1)
    setAt(idx, raw)
    if (raw && idx < length - 1) refs.current[idx + 1]?.focus()
  }

  function handleKeyDown(idx, e) {
    if (e.key === 'Backspace' && !value[idx] && idx > 0) {
      refs.current[idx - 1]?.focus()
    }
  }

  function handlePaste(e) {
    const pasted = e.clipboardData.getData('text').replace(/\D/g, '').slice(0, length)
    if (pasted) {
      e.preventDefault()
      onChange(pasted)
      refs.current[Math.min(pasted.length, length - 1)]?.focus()
    }
  }

  return (
    <div className="code-input" onPaste={handlePaste}>
      {Array.from({ length }).map((_, i) => (
        <input
          key={i}
          ref={(el) => (refs.current[i] = el)}
          className="code-input__cell"
          inputMode="numeric"
          maxLength={1}
          value={value[i] || ''}
          onChange={(e) => handleChange(i, e)}
          onKeyDown={(e) => handleKeyDown(i, e)}
        />
      ))}
    </div>
  )
}
