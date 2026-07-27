import { Activity, Clipboard, Save, X } from 'lucide-react'
import { useEffect, useState } from 'react'
import type { Preferences, ToolDiagnostic } from '../types'
import { IconButton, SuccessMark, Toggle } from './Primitives'

interface Props {
  open: boolean
  preferences: Preferences
  diagnostics: ToolDiagnostic[]
  checking: boolean
  onClose: () => void
  onSave: (preferences: Preferences) => void
  onCheckTools: (preferences: Preferences) => void
  onDoctor: (preferences: Preferences) => void
}

export function SettingsDrawer({ open, preferences, diagnostics, checking, onClose, onSave, onCheckTools, onDoctor }: Props) {
  const [draft, setDraft] = useState(preferences)
  useEffect(() => setDraft(preferences), [preferences, open])
  const set = <K extends keyof Preferences>(key: K, value: Preferences[K]) => setDraft((current) => ({ ...current, [key]: value }))
  return (
    <div className={`drawer-layer ${open ? 'open' : ''}`} aria-hidden={!open}>
      <button type="button" className="drawer-backdrop" aria-label="关闭设置背景" onClick={onClose} />
      <aside className="settings-drawer">
        <div className="drawer-header"><div><h2>设置</h2><p>下载行为与外部工具</p></div><IconButton label="关闭设置" onClick={onClose}><X size={19} /></IconButton></div>
        <div className="drawer-content">
          <section className="settings-section"><h3>队列</h3><Toggle checked={draft.autoQueue} onChange={(value) => set('autoQueue', value)} label="自动开始下一个任务" description="当前任务完成后继续等待队列" /></section>
          <section className="settings-section"><h3>下载</h3><div className="field"><span>同名文件</span><div className="segmented segmented--three">
            <button type="button" className={draft.fileExistsAction === 'rename' ? 'active' : ''} onClick={() => set('fileExistsAction', 'rename')}>追加流水号</button>
            <button type="button" className={draft.fileExistsAction === 'skip' ? 'active' : ''} onClick={() => set('fileExistsAction', 'skip')}>跳过</button>
            <button type="button" className={draft.fileExistsAction === 'overwrite' ? 'active' : ''} onClick={() => set('fileExistsAction', 'overwrite')}>覆盖</button>
          </div></div></section>
          <section className="settings-section"><h3>外观</h3><div className="segmented segmented--three">{(['system', 'light', 'dark'] as const).map((value) => <button type="button" key={value} className={draft.theme === value ? 'active' : ''} onClick={() => set('theme', value)}>{value === 'system' ? '跟随系统' : value === 'light' ? '浅色' : '深色'}</button>)}</div></section>
          <section className="settings-section"><div className="settings-title-row"><h3>外部工具</h3><button type="button" className="button button--secondary button--compact" disabled={checking} onClick={() => onCheckTools(draft)}><Activity size={16} />{checking ? '检测中' : '检测工具'}</button></div>
            <label className="field"><span>FFmpeg</span><input value={draft.ffmpegPath} onChange={(event) => set('ffmpegPath', event.target.value)} placeholder="自动探测或填写完整路径" /></label>
            <label className="field"><span>MP4Box</span><input value={draft.mp4boxPath} onChange={(event) => set('mp4boxPath', event.target.value)} placeholder="可选" /></label>
            <label className="field"><span>aria2c</span><input value={draft.aria2cPath} onChange={(event) => set('aria2cPath', event.target.value)} placeholder="可选" /></label>
            {diagnostics.length > 0 && <div className="diagnostic-list">{diagnostics.map((item) => <div className={`diagnostic-row ${item.found ? 'found' : 'missing'}`} key={item.name}>{item.found ? <SuccessMark /> : <span className="missing-mark">!</span>}<div><strong>{item.name}</strong><span>{item.found ? item.path : item.message}</span>{item.version && <small>{item.version}</small>}{!item.found && item.installHint && <small>{item.installHint}</small>}</div></div>)}</div>}
            <button type="button" className="button button--secondary button--wide" onClick={() => onDoctor(draft)}><Clipboard size={16} />复制完整诊断</button>
          </section>
        </div>
        <div className="drawer-footer"><button type="button" className="button button--secondary" onClick={onClose}>取消</button><button type="button" className="button button--primary" onClick={() => onSave(draft)}><Save size={16} />保存设置</button></div>
      </aside>
    </div>
  )
}
