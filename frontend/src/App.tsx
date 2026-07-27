import { useEffect, useMemo, useRef, useState } from 'react'
import { ClipboardSetText, EventsOn } from '../wailsjs/runtime/runtime'
import {
  CheckTools,
  ChooseWorkDir,
  ClearEndedTasks,
  CreateTask,
  DeleteTask,
  Doctor,
  FillFormData,
  GetBootstrap,
  GetTaskLog,
  OpenFile,
  OpenWorkDir,
  RevealFile,
  RetryFailedTasks,
  RetryTask,
  SavePreferences,
  StartLogin,
  StartNextPending,
  StartTask,
  StopLogin,
  StopTask,
} from '../wailsjs/go/main/App'
import { AppHeader } from './components/AppHeader'
import { CreatePanel } from './components/CreatePanel'
import { ConfirmDialog, LoginDialog, Toast } from './components/Overlays'
import { SettingsDrawer } from './components/SettingsDrawer'
import { TaskDetails } from './components/TaskDetails'
import { TaskQueue } from './components/TaskQueue'
import { mockBootstrap, mockLog } from './mock'
import type { Bootstrap, ConfirmState, LoginSession, Preferences, QueueSummary, Task, TaskInput, ToastTone, ToolDiagnostic } from './types'
import { errorMessage, messageTone } from './utils'

const defaultPreferences: Preferences = {
  workDir: '', selectPage: '', videoIndex: '', audioIndex: '', dfnPriority: '1080P 高清,720P 高清',
  encodingPriority: 'hevc,avc,av1', extraArgs: '', ffmpegPath: '', mp4boxPath: '', aria2cPath: '',
  fileExistsAction: 'skip',
  mode: '下载', channel: 'WEB', downloadDanmaku: false, skipSubtitle: false, skipCover: false,
  skipMux: false, useAria2c: false, autoQueue: true, theme: 'system',
}
const emptySummary: QueueSummary = { total: 0, pending: 0, running: 0, success: 0, failed: 0, stopped: 0 }

function App() {
  const [preferences, setPreferences] = useState(defaultPreferences)
  const [form, setForm] = useState<TaskInput>({ ...defaultPreferences, url: '' })
  const [tasks, setTasks] = useState<Task[]>([])
  const [summary, setSummary] = useState(emptySummary)
  const [selectedID, setSelectedID] = useState<number | null>(null)
  const selectedIDRef = useRef<number | null>(null)
  const [log, setLog] = useState('')
  const [loginOpen, setLoginOpen] = useState(false)
  const [loginSession, setLoginSession] = useState<LoginSession | null>(null)
  const loginIDRef = useRef(0)
  const [loginQRCode, setLoginQRCode] = useState('')
  const [version, setVersion] = useState('1.0.13')
  const [buildTime, setBuildTime] = useState('')
  const [status, setStatus] = useState('正在初始化...')
  const [toast, setToast] = useState<{ message: string; tone: ToastTone }>({ message: '', tone: 'neutral' })
  const [busy, setBusy] = useState(false)
  const [settingsOpen, setSettingsOpen] = useState(false)
  const [diagnostics, setDiagnostics] = useState<ToolDiagnostic[]>([])
  const [checkingTools, setCheckingTools] = useState(false)
  const [confirm, setConfirm] = useState<ConfirmState | null>(null)
  const [confirmBusy, setConfirmBusy] = useState(false)

  const selectedTask = useMemo(() => tasks.find((task) => task.id === selectedID), [tasks, selectedID])
  const isDark = useMemo(() => {
    if (preferences.theme === 'dark') return true
    if (preferences.theme === 'light') return false
    return window.matchMedia('(prefers-color-scheme: dark)').matches
  }, [preferences.theme])

  const notify = (message: string, tone = messageTone(message)) => {
    setStatus(message)
    setToast({ message, tone })
  }

  const upsertTask = (task: Task) => setTasks((current) => {
    const index = current.findIndex((item) => item.id === task.id)
    if (index < 0) return [...current, task]
    const next = [...current]
    next[index] = task
    return next
  })

  useEffect(() => {
    document.documentElement.dataset.theme = isDark ? 'dark' : 'light'
  }, [isDark])

  useEffect(() => {
    let active = true
    const hasWailsRuntime = Boolean((window as unknown as { go?: unknown }).go)
    if (!hasWailsRuntime) {
      setPreferences(mockBootstrap.preferences)
      setForm({ ...mockBootstrap.preferences, url: '' })
      setTasks(mockBootstrap.tasks)
      setSummary(mockBootstrap.summary)
      setVersion(mockBootstrap.version)
      setBuildTime(mockBootstrap.buildTime)
      setStatus(mockBootstrap.status)
      setSelectedID(mockBootstrap.tasks[0].id)
      setLog(mockLog)
      return () => { active = false }
    }
    GetBootstrap().then((raw) => {
      if (!active) return
      const data = raw as unknown as Bootstrap
      setPreferences(data.preferences)
      setForm({ ...data.preferences, url: '' })
      setTasks(data.tasks || [])
      setSummary(data.summary || emptySummary)
      setVersion(data.version)
      setBuildTime(data.buildTime)
      setStatus(data.status || '就绪')
      if (data.tasks?.length) setSelectedID(data.tasks[data.tasks.length - 1].id)
    }).catch((error) => notify(`初始化失败：${errorMessage(error)}`, 'danger'))

    const cancelAdded = EventsOn('task:added', (payload: Task) => { upsertTask(payload); setSelectedID(payload.id) })
    const cancelUpdated = EventsOn('task:updated', (payload: Task) => upsertTask(payload))
    const cancelFinished = EventsOn('task:finished', (payload: Task) => upsertTask(payload))
    const cancelDeleted = EventsOn('task:deleted', (payload: { taskID: number }) => setTasks((current) => current.filter((task) => task.id !== payload.taskID)))
    const cancelReset = EventsOn('tasks:reset', (payload: Task[]) => setTasks(payload || []))
    const cancelSummary = EventsOn('queue:summary', (payload: QueueSummary) => setSummary(payload))
    const cancelStatus = EventsOn('app:status', (payload: { message: string }) => setStatus(payload.message))
    const cancelLoginUpdated = EventsOn('login:updated', (payload: LoginSession) => { loginIDRef.current = payload.id; setLoginSession(payload) })
    const cancelLoginQR = EventsOn('login:qrcode', (payload: { id: number; dataURL: string }) => { if (loginIDRef.current === payload.id) setLoginQRCode(payload.dataURL || '') })
    const cancelLoginFinished = EventsOn('login:finished', (payload: LoginSession) => { if (loginIDRef.current === payload.id) setLoginSession(payload) })
    const cancelLogReset = EventsOn('task:log-reset', (payload: { taskID: number; log: string }) => { if (selectedIDRef.current === payload.taskID) setLog(payload.log || '') })
    const cancelLog = EventsOn('task:log', (payload: { taskID: number; line: string }) => { if (selectedIDRef.current === payload.taskID) setLog((current) => `${current}${payload.line}\n`) })
    return () => { active = false; cancelAdded(); cancelUpdated(); cancelFinished(); cancelDeleted(); cancelReset(); cancelSummary(); cancelStatus(); cancelLoginUpdated(); cancelLoginQR(); cancelLoginFinished(); cancelLogReset(); cancelLog() }
  }, [])

  useEffect(() => {
    selectedIDRef.current = selectedID
    setLog('')
    if (selectedID === null) return
    if (!(window as unknown as { go?: unknown }).go) {
      setLog(selectedID === mockBootstrap.tasks[0].id ? mockLog : '')
      return
    }
    GetTaskLog(selectedID).then(setLog).catch(() => setLog(''))
  }, [selectedID])

  useEffect(() => {
    if (!toast.message) return
    const timer = window.setTimeout(() => setToast((current) => ({ ...current, message: '' })), 3600)
    return () => window.clearTimeout(timer)
  }, [toast.message])

  const changeForm = <K extends keyof TaskInput>(key: K, value: TaskInput[K]) => setForm((current) => ({ ...current, [key]: value }))

  const run = async (work: () => Promise<unknown>, success?: string) => {
    setBusy(true)
    try { await work(); if (success) notify(success, 'success') } catch (error) { notify(errorMessage(error), 'danger') } finally { setBusy(false) }
  }

  const createTask = (startNow: boolean) => void run(async () => {
    if (!form.url.trim()) throw new Error('请输入视频地址')
    const result = await CreateTask(form, startNow) as unknown as Task
    upsertTask(result)
    setSelectedID(result.id)
    setPreferences(({ theme }) => ({ ...form, theme }))
    setForm((current) => ({ ...current, url: '' }))
  }, startNow ? '任务已创建并开始' : '任务已加入队列')

  const chooseDir = () => void run(async () => { const path = await ChooseWorkDir(); if (path) changeForm('workDir', path) })
  const startTask = (id: number) => void run(() => StartTask(id))
  const stopTask = (id: number) => void run(() => StopTask(id), '已请求停止任务')
  const retryTask = (id: number) => void run(async () => { const task = await RetryTask(id) as unknown as Task; upsertTask(task); setSelectedID(task.id) })
  const startNext = () => void run(() => StartNextPending())
  const retryFailed = () => void run(async () => { const added = await RetryFailedTasks() as unknown as Task[]; added.forEach(upsertTask); if (added.length) setSelectedID(added[0].id) }, '失败和停止任务已重新排队')
  const fillTask = (id: number) => void run(async () => { const input = await FillFormData(id) as unknown as TaskInput; setForm(input); notify('已填入下载表单', 'success') })
  const login = (kind: 'WEB' | 'TV') => {
    setLoginOpen(true)
    setLoginQRCode('')
    loginIDRef.current = 0
    if (!(window as unknown as { go?: unknown }).go) {
      setLoginSession({ id: 0, kind, status: '界面预览', message: '预览模式不执行真实登录', log: '', running: false, successful: false })
      return
    }
    setLoginSession({ id: 0, kind, status: '正在启动登录', message: '正在准备二维码', log: '', running: true, successful: false })
    void run(async () => {
      const session = await StartLogin(kind) as unknown as LoginSession
      loginIDRef.current = session.id
      setLoginSession(session)
    })
  }
  const closeLogin = () => {
    setLoginOpen(false)
    setLoginQRCode('')
    if (loginSession?.running) void StopLogin().catch((error) => notify(errorMessage(error), 'danger'))
  }

  const askDelete = (id: number) => setConfirm({ title: '删除任务', message: `确定删除任务 #${id} 吗？已下载文件不会被删除。`, confirmLabel: '删除任务', danger: true, action: async () => { await DeleteTask(id); if (selectedIDRef.current === id) setSelectedID(null); notify(`已删除任务 #${id}`, 'success') } })
  const askClear = () => setConfirm({ title: '清理已结束任务', message: '已完成、失败和停止的任务会从列表移除，下载文件会保留。', confirmLabel: '清理任务', danger: true, action: async () => { const count = await ClearEndedTasks(); notify(`已清理 ${count} 个任务`, 'success') } })
  const confirmAction = async () => { if (!confirm) return; setConfirmBusy(true); try { await confirm.action(); setConfirm(null) } catch (error) { notify(errorMessage(error), 'danger') } finally { setConfirmBusy(false) } }

  const copy = (text: string, message: string) => { if (!text) return; ClipboardSetText(text); notify(message, 'success') }
  const saveSettings = (next: Preferences) => void run(async () => { await SavePreferences(next); setPreferences(next); setForm((current) => ({ ...current, ...next, url: current.url })); setSettingsOpen(false) }, '设置已保存')
  const checkTools = (next: Preferences) => void (async () => { setCheckingTools(true); try { setDiagnostics(await CheckTools({ ffmpegPath: next.ffmpegPath, mp4boxPath: next.mp4boxPath, aria2cPath: next.aria2cPath }) as unknown as ToolDiagnostic[]) } catch (error) { notify(errorMessage(error), 'danger') } finally { setCheckingTools(false) } })()
  const doctor = (next: Preferences) => void run(async () => copy(await Doctor({ ffmpegPath: next.ffmpegPath, mp4boxPath: next.mp4boxPath, aria2cPath: next.aria2cPath }), '已复制完整诊断'))
  const toggleTheme = () => {
    const theme = isDark ? 'light' : 'dark'
    const next = { ...preferences, theme } as Preferences
    setPreferences(next)
    setForm((current) => ({ ...current, theme }))
    void SavePreferences(next).catch((error) => notify(errorMessage(error), 'danger'))
  }

  return (
    <div className="app-shell">
      <AppHeader summary={summary} version={version} dark={isDark} onToggleTheme={toggleTheme} onLogin={login} onOpenFolder={() => void run(() => OpenWorkDir(form.workDir), '已打开保存目录')} onOpenSettings={() => setSettingsOpen(true)} />
      <main className="workspace">
        <CreatePanel form={form} videoOptions={selectedTask?.videoOptions || []} audioOptions={selectedTask?.audioOptions || []} busy={busy} onChange={changeForm} onChooseDir={chooseDir} onCreate={createTask} />
        <TaskQueue tasks={tasks} selectedID={selectedID} onSelect={setSelectedID} onStart={startTask} onStop={stopTask} onRetry={retryTask} onStartNext={startNext} onRetryFailed={retryFailed} onClearEnded={askClear} />
        <TaskDetails task={selectedTask} log={log} onStart={startTask} onStop={stopTask} onRetry={retryTask} onFill={fillTask} onDelete={askDelete} onCopy={copy} onOpenFile={(path) => void run(() => OpenFile(path))} onRevealFile={(path) => void run(() => RevealFile(path))} />
      </main>
      <footer className="status-bar"><span className="status-dot" />{status}<span className="status-build">{buildTime && `构建 ${buildTime}`}</span></footer>
      <SettingsDrawer open={settingsOpen} preferences={preferences} diagnostics={diagnostics} checking={checkingTools} onClose={() => setSettingsOpen(false)} onSave={saveSettings} onCheckTools={checkTools} onDoctor={doctor} />
      <ConfirmDialog state={confirm} busy={confirmBusy} onCancel={() => setConfirm(null)} onConfirm={() => void confirmAction()} />
      <LoginDialog open={loginOpen} session={loginSession} qrCode={loginQRCode} busy={busy} onClose={closeLogin} onRetry={login} />
      <Toast message={toast.message} tone={toast.tone} onClose={() => setToast((current) => ({ ...current, message: '' }))} />
    </div>
  )
}

export default App
