import type { Bootstrap, Task } from './types'

const now = new Date()
const task = (source: Partial<Task> & Pick<Task, 'id' | 'url' | 'status'>): Task => {
  const { id, url, status, ...overrides } = source
  return {
  id,
  url,
  status,
  workDir: '/Users/long/Downloads/bilibili',
  mode: '下载',
  channel: 'WEB',
  selectPage: '',
  videoIndex: '',
  audioIndex: '',
  dfnPriority: '1080P 高清,720P 高清',
  encodingPriority: 'hevc,avc,av1',
  extraArgs: '',
  ffmpegPath: '/opt/homebrew/bin/ffmpeg',
  mp4boxPath: '',
  aria2cPath: '',
  fileExistsAction: 'skip',
  args: [],
  bytes: 0,
  progress: 0,
  files: [],
  createdAt: now.toISOString(),
  startedAt: now.toISOString(),
  endedAt: '',
  currentSpeed: 0,
  averageSpeed: 0,
  elapsedSeconds: 0,
  videoOptions: [],
  audioOptions: [],
  command: 'BB-DL https://www.bilibili.com/video/BV1J9EB6xEAB',
  actions: { canStart: false, canStop: false, canRetry: false, canDelete: true },
  ...overrides,
  }
}

export const mockBootstrap: Bootstrap = {
  preferences: {
    workDir: '/Users/long/Downloads/bilibili', selectPage: '', videoIndex: '', audioIndex: '',
    dfnPriority: '1080P 高清,720P 高清', encodingPriority: 'hevc,avc,av1', extraArgs: '',
    ffmpegPath: '/opt/homebrew/bin/ffmpeg', mp4boxPath: '', aria2cPath: '', fileExistsAction: 'skip', mode: '下载', channel: 'WEB',
    downloadDanmaku: false, skipSubtitle: false, skipCover: false, skipMux: false, useAria2c: false,
    autoQueue: true, theme: 'light',
  },
  tasks: [
    task({ id: 18, url: 'https://www.bilibili.com/video/BV1J9EB6xEAB', status: '下载中', bytes: 684510412, progress: 0.64, currentSpeed: 8420102, averageSpeed: 7213400, elapsedSeconds: 86, actions: { canStart: false, canStop: true, canRetry: false, canDelete: false }, videoOptions: ['0. 1920x1080 HEVC', '1. 1920x1080 AVC', '2. 1280x720 AVC'], audioOptions: ['0. 192K AAC', '1. 132K AAC'] }),
    task({ id: 17, url: 'BV1J9EB6xEAB · 仅查看流信息', status: '已完成', mode: '仅查看', progress: 1, endedAt: now.toISOString() }),
    task({ id: 16, url: 'BV1J9EB6xEAB · 视频下载', status: '失败', progress: 0.22, bytes: 12840122, actions: { canStart: true, canStop: false, canRetry: true, canDelete: true } }),
  ],
  summary: { total: 3, pending: 0, running: 1, success: 1, failed: 1, stopped: 0 },
  version: '1.0.12',
  buildTime: '2026-07-26T10:58:00Z',
  status: '任务 #18 正在下载',
}

export const mockLog = `$ BB-DL https://www.bilibili.com/video/BV1J9EB6xEAB
获取aid中...
获取aid结束: 114784550780185
获取视频信息中...
视频标题: 下载器测试视频
共计3条视频流.
  0. 1920x1080 HEVC
  1. 1920x1080 AVC
  2. 1280x720 AVC
共计2条音频流.
  0. 192K AAC
  1. 132K AAC
开始解析P1: 下载器测试视频 (1 of 1)
开始下载P1视频...
当前速度 8.03 MiB/s
`
