import { FolderOpen, LogIn, Moon, Settings2, Sun } from 'lucide-react'
import type { QueueSummary } from '../types'
import { IconButton } from './Primitives'

interface Props {
  summary: QueueSummary
  version: string
  dark: boolean
  onToggleTheme: () => void
  onLogin: (kind: 'WEB' | 'TV') => void
  onOpenFolder: () => void
  onOpenSettings: () => void
}

export function AppHeader({ summary, version, dark, onToggleTheme, onLogin, onOpenFolder, onOpenSettings }: Props) {
  return (
    <header className="app-header">
      <div className="brand">
        <span className="brand__mark">B</span>
        <span className="brand__copy"><strong>BB-DL</strong><small>v{version}</small></span>
      </div>
      <div className="queue-summary" aria-label="任务摘要">
        <span>共 {summary.total}</span>
        <span className="summary-running">下载 {summary.running}</span>
        <span className="summary-success">完成 {summary.success}</span>
        <span className="summary-danger">失败 {summary.failed}</span>
      </div>
      <div className="header-actions">
        <button className="header-command" type="button" title="WEB 登录" onClick={() => onLogin('WEB')}><LogIn size={16} />WEB 登录</button>
        <button className="header-command" type="button" title="TV 登录" onClick={() => onLogin('TV')}><LogIn size={16} />TV 登录</button>
        <span className="header-divider" />
        <IconButton label="打开保存目录" onClick={onOpenFolder}><FolderOpen size={18} /></IconButton>
        <IconButton label={dark ? '切换浅色主题' : '切换深色主题'} onClick={onToggleTheme}>{dark ? <Sun size={18} /> : <Moon size={18} />}</IconButton>
        <IconButton label="设置" onClick={onOpenSettings}><Settings2 size={18} /></IconButton>
      </div>
    </header>
  )
}
