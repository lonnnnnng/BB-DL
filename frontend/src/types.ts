export type FileExistsAction = 'rename' | 'skip' | 'overwrite'

export interface Preferences {
  workDir: string
  selectPage: string
  videoIndex: string
  audioIndex: string
  dfnPriority: string
  encodingPriority: string
  extraArgs: string
  ffmpegPath: string
  mp4boxPath: string
  aria2cPath: string
  fileExistsAction: FileExistsAction
  mode: string
  channel: string
  downloadDanmaku: boolean
  skipSubtitle: boolean
  skipCover: boolean
  skipMux: boolean
  useAria2c: boolean
  autoQueue: boolean
  theme: 'system' | 'light' | 'dark'
}

export interface TaskInput extends Preferences {
  url: string
}

export interface ManagedFile {
  path: string
  relPath: string
  size: number
  modTime: string
}

export interface TaskActions {
  canStart: boolean
  canStop: boolean
  canRetry: boolean
  canDelete: boolean
}

export interface Task {
  id: number
  url: string
  workDir: string
  mode: string
  channel: string
  selectPage: string
  videoIndex: string
  audioIndex: string
  dfnPriority: string
  encodingPriority: string
  extraArgs: string
  ffmpegPath: string
  mp4boxPath: string
  aria2cPath: string
  fileExistsAction: FileExistsAction
  args: string[]
  status: string
  bytes: number
  progress: number
  files: ManagedFile[]
  createdAt: string
  startedAt: string
  endedAt: string
  currentSpeed: number
  averageSpeed: number
  elapsedSeconds: number
  videoOptions: string[]
  audioOptions: string[]
  command: string
  actions: TaskActions
}

export interface LoginSession {
  id: number
  kind: 'WEB' | 'TV'
  status: string
  message: string
  log: string
  running: boolean
  successful: boolean
}

export interface QueueSummary {
  total: number
  pending: number
  running: number
  success: number
  failed: number
  stopped: number
}

export interface Bootstrap {
  preferences: Preferences
  tasks: Task[]
  summary: QueueSummary
  version: string
  buildTime: string
  status: string
}

export interface ToolDiagnostic {
  name: string
  found: boolean
  path: string
  version: string
  message: string
  installHint: string
}

export type ToastTone = 'neutral' | 'success' | 'warning' | 'danger'

export interface ConfirmState {
  title: string
  message: string
  confirmLabel: string
  danger?: boolean
  action: () => Promise<void>
}
