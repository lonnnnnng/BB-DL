import { Clipboard, Download, ExternalLink, FileSearch, Files, FolderOpen, ListVideo, Play, RotateCcw, Search, Square, TerminalSquare, Trash2 } from 'lucide-react'
import { useEffect, useMemo, useRef, useState } from 'react'
import type { ManagedFile, Task } from '../types'
import { formatBytes, formatDate, formatDuration, taskTitle } from '../utils'
import { EmptyState, IconButton, StatusBadge, Toggle } from './Primitives'

interface Props {
  task?: Task
  log: string
  onStart: (id: number) => void
  onStop: (id: number) => void
  onRetry: (id: number) => void
  onFill: (id: number) => void
  onDelete: (id: number) => void
  onCopy: (text: string, message: string) => void
  onOpenFile: (path: string) => void
  onRevealFile: (path: string) => void
}

function lineTone(line: string): string {
  if (/失败|错误|error|fatal|找不到/i.test(line)) return 'danger'
  if (/完成|成功|完毕|已登录/.test(line)) return 'success'
  if (/警告|提示|重试|停止|warning/i.test(line)) return 'warning'
  if (/^\$ |开始|下载|解析|获取/.test(line)) return 'accent'
  return 'default'
}

function FileRow({ file, selected, onSelect }: { file: ManagedFile; selected: boolean; onSelect: () => void }) {
  return <button type="button" className={`file-row ${selected ? 'selected' : ''}`} onClick={onSelect}><span className="file-icon"><Download size={16} /></span><span className="file-copy"><strong>{file.relPath}</strong><small>{formatBytes(file.size)} · {formatDate(file.modTime)}</small></span></button>
}

export function TaskDetails({ task, log, onStart, onStop, onRetry, onFill, onDelete, onCopy, onOpenFile, onRevealFile }: Props) {
  const [tab, setTab] = useState<'log' | 'files'>('log')
  const [follow, setFollow] = useState(true)
  const [logQuery, setLogQuery] = useState('')
  const [selectedFile, setSelectedFile] = useState<number>(0)
  const logRef = useRef<HTMLDivElement>(null)
  const lines = useMemo(() => {
    const all = log.split('\n')
    const filtered = logQuery ? all.filter((line) => line.toLowerCase().includes(logQuery.toLowerCase())) : all
    return filtered.slice(-4000)
  }, [log, logQuery])

  useEffect(() => { setSelectedFile(0) }, [task?.id])
  useEffect(() => { if (follow && logRef.current) logRef.current.scrollTop = logRef.current.scrollHeight }, [log, follow])

  if (!task) return <section className="details-panel"><EmptyState icon={<ListVideo size={28} />} title="选择一个任务" description="查看进度、运行日志和下载完成文件" /></section>
  const file = task.files?.[selectedFile]
  const percent = Math.max(0, Math.min(100, task.progress * 100))

  return (
    <section className="details-panel">
      <div className="details-header">
        <div className="details-title-row"><div className="details-title"><span>任务 #{task.id}</span><h2>{taskTitle(task.url)}</h2></div><StatusBadge status={task.status} /></div>
        <div className="details-subtitle"><span>{task.mode}</span><span>{task.channel}</span><span>创建 {formatDate(task.createdAt)}</span></div>
        <div className="details-progress"><div className="details-progress__labels"><strong>{percent.toFixed(0)}%</strong><span>{task.status === '下载中' ? '正在下载' : task.status}</span></div><div className="progress-track progress-track--large"><span style={{ width: `${percent}%` }} /></div></div>
        <div className="metrics-row"><div><small>已下载</small><strong>{formatBytes(task.bytes)}</strong></div><div><small>当前速度</small><strong>{formatBytes(task.currentSpeed)}/s</strong></div><div><small>平均速度</small><strong>{formatBytes(task.averageSpeed)}/s</strong></div><div><small>耗时</small><strong>{formatDuration(task.elapsedSeconds)}</strong></div></div>
        <div className="detail-actions">
          {task.actions.canStart && <button type="button" className="button button--primary button--compact" onClick={() => onStart(task.id)}><Play size={16} fill="currentColor" />开始</button>}
          {task.actions.canStop && <button type="button" className="button button--danger button--compact" onClick={() => onStop(task.id)}><Square size={15} />停止</button>}
          {task.actions.canRetry && <button type="button" className="button button--secondary button--compact" onClick={() => onRetry(task.id)}><RotateCcw size={16} />重试</button>}
          {task.channel !== '账号' && <button type="button" className="button button--secondary button--compact" onClick={() => onFill(task.id)}><FileSearch size={16} />填入表单</button>}
          <span className="action-spacer" />
          {task.command && <IconButton label="复制命令" onClick={() => onCopy(task.command, '已复制任务命令')}><Clipboard size={17} /></IconButton>}
          {task.actions.canDelete && <IconButton label="删除任务" variant="danger" onClick={() => onDelete(task.id)}><Trash2 size={17} /></IconButton>}
        </div>
      </div>

      <div className="detail-tabs"><button type="button" className={tab === 'log' ? 'active' : ''} onClick={() => setTab('log')}><TerminalSquare size={16} />运行日志</button><button type="button" className={tab === 'files' ? 'active' : ''} onClick={() => setTab('files')}><Files size={16} />完成文件 <span>{task.files?.length || 0}</span></button></div>
      {tab === 'log' ? (
        <div className="log-pane">
          <div className="log-toolbar"><label className="search-field"><Search size={15} /><input value={logQuery} onChange={(event) => setLogQuery(event.target.value)} placeholder="筛选日志" /></label><Toggle checked={follow} onChange={setFollow} label="跟随末尾" /><IconButton label="复制日志" onClick={() => onCopy(log, '已复制运行日志')} disabled={!log}><Clipboard size={17} /></IconButton></div>
          <div className="log-viewer" ref={logRef}>{lines.map((line, index) => <div key={`${index}-${line.slice(0, 16)}`} className={`log-line log-line--${lineTone(line)}`}><span className="log-line__number">{index + 1}</span><code>{line || ' '}</code></div>)}</div>
        </div>
      ) : (
        <div className="files-pane">
          <div className="files-toolbar"><span>{task.files?.length ? `${task.files.length} 个输出文件` : '任务完成后会在这里显示文件'}</span><div>{file && <><IconButton label="打开文件" onClick={() => onOpenFile(file.path)}><ExternalLink size={17} /></IconButton><IconButton label="在文件管理器中定位" onClick={() => onRevealFile(file.path)}><FolderOpen size={17} /></IconButton><IconButton label="复制文件路径" onClick={() => onCopy(file.path, '已复制文件路径')}><Clipboard size={17} /></IconButton></>}{task.files?.length > 0 && <IconButton label="复制全部文件路径" onClick={() => onCopy(task.files.map((item) => item.path).join('\n'), `已复制 ${task.files.length} 个文件路径`)}><Files size={17} /></IconButton>}</div></div>
          <div className="file-list">{task.files?.length ? task.files.map((item, index) => <FileRow key={item.path} file={item} selected={selectedFile === index} onSelect={() => setSelectedFile(index)} />) : <EmptyState icon={<Files size={24} />} title="暂无完成文件" description="运行下载任务后，输出文件会自动归集到这里" />}</div>
        </div>
      )}
    </section>
  )
}
