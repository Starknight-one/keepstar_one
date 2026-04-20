import React from 'react'

export default function PillButton({
  variant = 'primary',
  block = false,
  type = 'button',
  children,
  className = '',
  ...rest
}) {
  const classes = [
    'pill-btn',
    `pill-btn--${variant}`,
    block ? 'pill-btn--block' : '',
    className,
  ].filter(Boolean).join(' ')

  return (
    <button type={type} className={classes} {...rest}>
      {children}
    </button>
  )
}
