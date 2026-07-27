import { ChevronDown, FolderOpen, ListPlus, Play, SlidersHorizontal } from 'lucide-react'
import type { TaskInput } from '../types'
import { IconButton, Toggle } from './Primitives'

const modes = ['下载', '仅查看', '仅视频', '仅音频', '仅封面', '仅字幕', '仅弹幕']
const channels = ['WEB', 'TV', 'APP', '国际版']

interface Props {
  form: TaskInput
  videoOptions: string[]
  audioOptions: string[]
  busy: boolean
  onChange: <K extends keyof TaskInput>(key: K, value: TaskInput[K]) => void
  onChooseDir: () => void
  onCreate: (startNow: boolean) => void
}

function StreamField({ label, value, options, onChange }: { label: string; value: string; options: string[]; onChange: (value: string) => void }) {
  if (options.length > 0) {
    return <label className="field"><span>{label}</span><select value={value || '自动'} onChange={(event) => onChange(event.target.value)}><option>自动</option>{options.map((option) => <option key={option} value={option}>{option}</option>)}</select></label>
  }
  return <label className="field"><span>{label}</span><input value={value} onChange={(event) => onChange(event.target.value)} placeholder="自动，或填写序号" /></label>
}

export function CreatePanel({ form, videoOptions, audioOptions, busy, onChange, onChooseDir, onCreate }: Props) {
  return (
    <aside className="create-panel">
      <div className="panel-heading"><div><h2>新建任务</h2><p>创建下载或解析任务</p></div><SlidersHorizontal size={18} /></div>
      <div className="create-form">
        <label className="field field--prominent">
          <span>视频地址</span>
          <textarea rows={3} value={form.url} onChange={(event) => onChange('url', event.target.value)} placeholder="BV / av / ep / URL" autoFocus />
        </label>
        <label className="field"><span>保存目录</span><div className="input-action"><input value={form.workDir} onChange={(event) => onChange('workDir', event.target.value)} /><IconButton label="选择目录" onClick={onChooseDir}><FolderOpen size={17} /></IconButton></div></label>
        <div className="field"><span>下载模式</span><select value={form.mode} onChange={(event) => onChange('mode', event.target.value)}>{modes.map((mode) => <option key={mode}>{mode}</option>)}</select></div>
        <div className="field"><span>接口</span><div className="segmented segmented--four">{channels.map((channel) => <button type="button" key={channel} className={form.channel === channel ? 'active' : ''} onClick={() => onChange('channel', channel)}>{channel}</button>)}</div></div>
        <div className="create-actions">
          <button type="button" className="button button--secondary" disabled={busy} onClick={() => onCreate(false)}><ListPlus size={17} />加入队列</button>
          <button type="button" className="button button--primary" disabled={busy} onClick={() => onCreate(true)}><Play size={17} fill="currentColor" />创建并开始</button>
        </div>
        <details className="options-section" open>
          <summary><span>下载设置</span><ChevronDown size={16} /></summary>
          <div className="options-content">
            <label className="field"><span>分 P</span><input value={form.selectPage} onChange={(event) => onChange('selectPage', event.target.value)} placeholder="1,2,LAST" /></label>
            <div className="field-grid"><StreamField label="视频流" value={form.videoIndex} options={videoOptions} onChange={(value) => onChange('videoIndex', value)} /><StreamField label="音频流" value={form.audioIndex} options={audioOptions} onChange={(value) => onChange('audioIndex', value)} /></div>
            <label className="field"><span>清晰度优先级</span><input value={form.dfnPriority} onChange={(event) => onChange('dfnPriority', event.target.value)} /></label>
            <label className="field"><span>编码优先级</span><input value={form.encodingPriority} onChange={(event) => onChange('encodingPriority', event.target.value)} /></label>
            <div className="field"><span>同名文件</span><div className="segmented segmented--three">
              <button type="button" className={form.fileExistsAction === 'rename' ? 'active' : ''} onClick={() => onChange('fileExistsAction', 'rename')}>追加流水号</button>
              <button type="button" className={form.fileExistsAction === 'skip' ? 'active' : ''} onClick={() => onChange('fileExistsAction', 'skip')}>跳过</button>
              <button type="button" className={form.fileExistsAction === 'overwrite' ? 'active' : ''} onClick={() => onChange('fileExistsAction', 'overwrite')}>覆盖</button>
            </div></div>
            <div className="toggle-list compact">
              <Toggle checked={form.downloadDanmaku} onChange={(value) => onChange('downloadDanmaku', value)} label="下载弹幕 XML/ASS" />
              <Toggle checked={form.skipSubtitle} onChange={(value) => onChange('skipSubtitle', value)} label="跳过字幕" />
              <Toggle checked={form.skipCover} onChange={(value) => onChange('skipCover', value)} label="跳过封面" />
              <Toggle checked={form.skipMux} onChange={(value) => onChange('skipMux', value)} label="跳过混流" />
              <Toggle checked={form.useAria2c} onChange={(value) => onChange('useAria2c', value)} label="使用 aria2c" />
            </div>
          </div>
        </details>
        <details className="options-section">
          <summary><span>额外参数</span><ChevronDown size={16} /></summary>
          <div className="options-content"><label className="field"><span>CLI 参数</span><textarea rows={4} value={form.extraArgs} onChange={(event) => onChange('extraArgs', event.target.value)} placeholder="--file-pattern '<videoTitle>[<dfn>]'" /></label></div>
        </details>
      </div>
    </aside>
  )
}
