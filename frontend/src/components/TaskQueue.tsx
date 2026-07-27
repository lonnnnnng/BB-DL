import { ListRestart, ListVideo, Play, RotateCcw, Search, SkipForward, Square, Trash2 } from 'lucide-react'
import { useMemo, useState } from 'react'
import type { Task } from '../types'
import { formatBytes, taskTitle } from '../utils'
import { EmptyState, IconButton, StatusBadge } from './Primitives'

interface Props {
  tasks: Task[]
  selectedID: number | null
  onSelect: (id: number) => void
  onStart: (id: number) => void
  onStop: (id: number) => void
  onRetry: (id: number) => void
  onStartNext: () => void
  onRetryFailed: () => void
  onClearEnded: () => void
}

export function TaskQueue({ tasks, selectedID, onSelect, onStart, onStop, onRetry, onStartNext, onRetryFailed, onClearEnded }: Props) {
  const [filter, setFilter] = useState<'all' | 'active' | 'ended'>('all')
  const [query, setQuery] = useState('')
  const visible = useMemo(() => tasks.filter((task) => {
    if (filter === 'active' && !['等待中', '下载中', '停止中'].includes(task.status)) return false
    if (filter === 'ended' && ['等待中', '下载中', '停止中'].includes(task.status)) return false
    return `${task.id} ${task.url} ${task.mode}`.toLowerCase().includes(query.trim().toLowerCase())
  }), [tasks, filter, query])
  const selected = tasks.find((task) => task.id === selectedID)

  return (
    <section className="queue-panel">
      <div className="panel-heading queue-heading"><div><h2>任务队列</h2><p>{tasks.length ? `${tasks.length} 个任务` : '暂无任务'}</p></div><div className="queue-tools">{selected?.actions.canStart && <IconButton label="开始选中任务" onClick={() => onStart(selected.id)}><Play size={17} /></IconButton>}{selected?.actions.canStop && <IconButton label="停止选中任务" variant="danger" onClick={() => onStop(selected.id)}><Square size={16} /></IconButton>}{selected?.actions.canRetry && <IconButton label="重新执行" onClick={() => onRetry(selected.id)}><RotateCcw size={17} /></IconButton>}<IconButton label="开始下一个等待任务" onClick={onStartNext}><SkipForward size={17} /></IconButton><IconButton label="批量重试失败和停止任务" onClick={onRetryFailed}><ListRestart size={17} /></IconButton><IconButton label="清理已结束任务" onClick={onClearEnded}><Trash2 size={17} /></IconButton></div></div>
      <div className="queue-filter"><div className="segmented">{([['all', '全部'], ['active', '进行中'], ['ended', '已结束']] as const).map(([value, label]) => <button type="button" key={value} className={filter === value ? 'active' : ''} onClick={() => setFilter(value)}>{label}</button>)}</div><label className="search-field"><Search size={15} /><input value={query} onChange={(event) => setQuery(event.target.value)} placeholder="搜索" /></label></div>
      <div className="task-list">
        {visible.length === 0 ? <EmptyState icon={<ListVideo size={24} />} title="没有匹配的任务" description="新建任务后会在这里显示下载状态" /> : visible.map((task) => (
          <button type="button" key={task.id} className={`task-row ${selectedID === task.id ? 'selected' : ''}`} onClick={() => onSelect(task.id)} onDoubleClick={() => task.actions.canStart && onStart(task.id)}>
            <div className="task-row__top"><span className="task-id">#{task.id}</span><StatusBadge status={task.status} /></div>
            <strong className="task-title">{taskTitle(task.url)}</strong>
            <div className="task-meta"><span>{task.mode}</span><span>{formatBytes(task.bytes)}</span>{task.currentSpeed > 0 && <span>{formatBytes(task.currentSpeed)}/s</span>}</div>
            <div className="progress-track"><span style={{ width: `${Math.max(0, Math.min(100, task.progress * 100))}%` }} /></div>
          </button>
        ))}
      </div>
    </section>
  )
}
