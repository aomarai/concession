import type { ButtonHTMLAttributes } from 'react'
import { usePop } from '../lib/spring'
import Icon, { type IconName } from './Icon'

export interface ButtonProps extends ButtonHTMLAttributes<HTMLButtonElement> {
  /** primary (velvet): the one main action; secondary (white pill); dark (ink pill); quiet: toolbars; danger: destructive; glass: on a poster. */
  variant?: 'primary' | 'secondary' | 'dark' | 'quiet' | 'danger' | 'glass'
  size?: 'm' | 's'
  icon?: IconName
}

export function buttonClass(variant: ButtonProps['variant'] = 'primary', size: ButtonProps['size'] = 'm', extra?: string) {
  return ['cn-btn', 'pop', variant !== 'primary' && `cn-btn--${variant}`, size === 's' && 'cn-btn--s', extra].filter(Boolean).join(' ')
}

// A pill button that sits on a hard shadow and pops on hover and press.
export default function Button({ variant, size, icon, className, children, type = 'button', ...rest }: ButtonProps) {
  const pop = usePop<HTMLButtonElement>()
  const iconOnly = icon !== undefined && children === undefined
  return (
    <button {...rest} {...pop} type={type} className={buttonClass(variant, size, [iconOnly && 'cn-btn--icon', className].filter(Boolean).join(' '))}>
      {icon && <Icon name={icon} />}
      {children}
    </button>
  )
}
