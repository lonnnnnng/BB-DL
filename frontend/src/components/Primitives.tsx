import type { ButtonHTMLAttributes, ReactNode } from 'react'
import { Check, LoaderCircle } from 'lucide-react'

interface IconButtonProps extends ButtonHTMLAttributes<HTMLButtonElement> {
  label: string
  children: ReactNode
  variant?: 'ghost' | 'soft' | 'danger'
}

export function IconButton({ label, children, variant = 'ghost', className = '', ...props }: IconButtonProps) {
  return (
    <button
      type="button"
      className={`icon-button icon-button--${variant} ${className}`}
      title={label}
      aria-label={label}
      {...props}
    >
      {children}
    </button>
  )
}

export function Toggle({ checked, onChange, label, description }: { checked: boolean; onChange: (checked: boolean) => void; label: string; description?: string }) {
  return (
    <label className="toggle-row">
      <span>
        <strong>{label}</strong>
        {description && <small>{description}</small>}
      </span>
      <input type="checkbox" checked={checked} onChange={(event) => onChange(event.target.checked)} />
      <span className="toggle-track" aria-hidden="true"><span /></span>
    </label>
  )
}

export function StatusBadge({ status }: { status: string }) {
  const tone = status === '已完成' ? 'success' : status === '失败' ? 'danger' : status === '下载中' || status === '停止中' ? 'warning' : 'neutral'
  return <span className={`status-badge status-badge--${tone}`}>{status}</span>
}

export function EmptyState({ icon, title, description }: { icon: ReactNode; title: string; description: string }) {
  return <div className="empty-state"><span className="empty-state__icon">{icon}</span><strong>{title}</strong><p>{description}</p></div>
}

export function BusyLabel({ children }: { children: ReactNode }) {
  return <><LoaderCircle size={15} className="spin" />{children}</>
}

export function SuccessMark() {
  return <span className="success-mark"><Check size={13} /></span>
}
