import { AlertTriangle, CheckCircle2, Info, LoaderCircle, QrCode, RotateCcw, X, XCircle } from 'lucide-react'
import type { ConfirmState, LoginSession, ToastTone } from '../types'
import { IconButton } from './Primitives'

export function ConfirmDialog({ state, busy, onCancel, onConfirm }: { state: ConfirmState | null; busy: boolean; onCancel: () => void; onConfirm: () => void }) {
  if (!state) return null
  return <div className="modal-layer"><button type="button" className="modal-backdrop" aria-label="取消" onClick={onCancel} /><div className="confirm-dialog" role="dialog" aria-modal="true" aria-labelledby="confirm-title"><span className={`confirm-icon ${state.danger ? 'danger' : ''}`}><AlertTriangle size={22} /></span><div><h2 id="confirm-title">{state.title}</h2><p>{state.message}</p></div><div className="confirm-actions"><button type="button" className="button button--secondary" onClick={onCancel}>取消</button><button type="button" className={`button ${state.danger ? 'button--danger' : 'button--primary'}`} disabled={busy} onClick={onConfirm}>{state.confirmLabel}</button></div></div></div>
}

export function Toast({ message, tone, onClose }: { message: string; tone: ToastTone; onClose: () => void }) {
  if (!message) return null
  const Icon = tone === 'success' ? CheckCircle2 : tone === 'danger' ? XCircle : tone === 'warning' ? AlertTriangle : Info
  return <div className={`toast toast--${tone}`} role="status"><Icon size={18} /><span>{message}</span><IconButton label="关闭提示" onClick={onClose}><X size={15} /></IconButton></div>
}

export function LoginDialog({ open, session, qrCode, busy, onClose, onRetry }: { open: boolean; session: LoginSession | null; qrCode: string; busy: boolean; onClose: () => void; onRetry: (kind: 'WEB' | 'TV') => void }) {
  if (!open) return null
  const kind = session?.kind || 'WEB'
  const lines = (session?.log || '').split('\n').filter(Boolean).slice(-5)
  return (
    <div className="modal-layer">
      <button type="button" className="modal-backdrop" aria-label="关闭登录" onClick={onClose} />
      <div className="login-dialog" role="dialog" aria-modal="true" aria-labelledby="login-title">
        <div className="login-dialog__header">
          <div><span>账号登录</span><h2 id="login-title">{kind} 登录</h2></div>
          <IconButton label="关闭登录" onClick={onClose}><X size={17} /></IconButton>
        </div>
        <div className={`login-qr ${session?.successful ? 'success' : ''}`}>
          {session?.successful ? <CheckCircle2 size={54} /> : qrCode ? <img src={qrCode} alt="Bilibili 登录二维码" /> : session?.running ? <LoaderCircle className="spin" size={34} /> : <QrCode size={38} />}
        </div>
        <div className="login-copy">
          <strong>{session?.status || '正在启动登录'}</strong>
          <span>{session?.message || '正在准备二维码'}</span>
        </div>
        {lines.length > 0 && <div className="login-log">{lines.map((line, index) => <div key={`${index}-${line}`}>{line}</div>)}</div>}
        <div className="login-actions">
          {!session?.running && !session?.successful && <button type="button" className="button button--primary" disabled={busy} onClick={() => onRetry(kind)}><RotateCcw size={16} />重新生成</button>}
          <button type="button" className="button button--secondary" onClick={onClose}>{session?.running ? '取消登录' : '关闭'}</button>
        </div>
      </div>
    </div>
  )
}
