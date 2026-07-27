import type { ToastTone } from './types'

export function formatBytes(value: number): string {
  const units = ['B', 'KiB', 'MiB', 'GiB', 'TiB']
  let size = Math.max(0, value || 0)
  let unit = 0
  while (size >= 1024 && unit < units.length - 1) {
    size /= 1024
    unit += 1
  }
  return unit === 0 ? `${Math.round(size)} ${units[unit]}` : `${size.toFixed(2)} ${units[unit]}`
}

export function formatDuration(seconds: number): string {
  const total = Math.max(0, Math.round(seconds || 0))
  const hours = Math.floor(total / 3600)
  const minutes = Math.floor((total % 3600) / 60)
  const rest = total % 60
  return hours > 0
    ? `${hours}:${String(minutes).padStart(2, '0')}:${String(rest).padStart(2, '0')}`
    : `${String(minutes).padStart(2, '0')}:${String(rest).padStart(2, '0')}`
}

export function formatDate(value: string): string {
  if (!value || value.startsWith('0001-')) return '-'
  const date = new Date(value)
  return Number.isNaN(date.getTime()) ? '-' : date.toLocaleString('zh-CN', { hour12: false })
}

export function taskTitle(url: string): string {
  return url || '未命名任务'
}

export function statusTone(status: string): ToastTone {
  if (status === '已完成') return 'success'
  if (status === '失败') return 'danger'
  if (status === '下载中' || status === '停止中') return 'warning'
  return 'neutral'
}

export function messageTone(message: string): ToastTone {
  if (/失败|错误|未找到|不可用|请输入/.test(message)) return 'danger'
  if (/完成|已复制|已打开|已保存|已加入/.test(message)) return 'success'
  if (/停止|等待|运行中/.test(message)) return 'warning'
  return 'neutral'
}

export function errorMessage(error: unknown): string {
  if (error instanceof Error) return error.message
  return String(error)
}
