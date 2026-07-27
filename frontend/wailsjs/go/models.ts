export namespace main {
	
	export class QueueSummary {
	    total: number;
	    pending: number;
	    running: number;
	    success: number;
	    failed: number;
	    stopped: number;
	
	    static createFrom(source: any = {}) {
	        return new QueueSummary(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.total = source["total"];
	        this.pending = source["pending"];
	        this.running = source["running"];
	        this.success = source["success"];
	        this.failed = source["failed"];
	        this.stopped = source["stopped"];
	    }
	}
	export class TaskActions {
	    canStart: boolean;
	    canStop: boolean;
	    canRetry: boolean;
	    canDelete: boolean;
	
	    static createFrom(source: any = {}) {
	        return new TaskActions(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.canStart = source["canStart"];
	        this.canStop = source["canStop"];
	        this.canRetry = source["canRetry"];
	        this.canDelete = source["canDelete"];
	    }
	}
	export class ManagedFile {
	    path: string;
	    relPath: string;
	    size: number;
	    // Go type: time
	    modTime: any;
	
	    static createFrom(source: any = {}) {
	        return new ManagedFile(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.path = source["path"];
	        this.relPath = source["relPath"];
	        this.size = source["size"];
	        this.modTime = this.convertValues(source["modTime"], null);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class TaskDTO {
	    id: number;
	    url: string;
	    workDir: string;
	    mode: string;
	    channel: string;
	    selectPage: string;
	    videoIndex: string;
	    audioIndex: string;
	    dfnPriority: string;
	    encodingPriority: string;
	    extraArgs: string;
	    ffmpegPath: string;
	    mp4boxPath: string;
	    aria2cPath: string;
	    fileExistsAction: string;
	    args: string[];
	    status: string;
	    bytes: number;
	    progress: number;
	    files: ManagedFile[];
	    // Go type: time
	    createdAt: any;
	    // Go type: time
	    startedAt: any;
	    // Go type: time
	    endedAt: any;
	    currentSpeed: number;
	    averageSpeed: number;
	    elapsedSeconds: number;
	    videoOptions: string[];
	    audioOptions: string[];
	    command: string;
	    actions: TaskActions;
	
	    static createFrom(source: any = {}) {
	        return new TaskDTO(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.url = source["url"];
	        this.workDir = source["workDir"];
	        this.mode = source["mode"];
	        this.channel = source["channel"];
	        this.selectPage = source["selectPage"];
	        this.videoIndex = source["videoIndex"];
	        this.audioIndex = source["audioIndex"];
	        this.dfnPriority = source["dfnPriority"];
	        this.encodingPriority = source["encodingPriority"];
	        this.extraArgs = source["extraArgs"];
	        this.ffmpegPath = source["ffmpegPath"];
	        this.mp4boxPath = source["mp4boxPath"];
	        this.aria2cPath = source["aria2cPath"];
	        this.fileExistsAction = source["fileExistsAction"];
	        this.args = source["args"];
	        this.status = source["status"];
	        this.bytes = source["bytes"];
	        this.progress = source["progress"];
	        this.files = this.convertValues(source["files"], ManagedFile);
	        this.createdAt = this.convertValues(source["createdAt"], null);
	        this.startedAt = this.convertValues(source["startedAt"], null);
	        this.endedAt = this.convertValues(source["endedAt"], null);
	        this.currentSpeed = source["currentSpeed"];
	        this.averageSpeed = source["averageSpeed"];
	        this.elapsedSeconds = source["elapsedSeconds"];
	        this.videoOptions = source["videoOptions"];
	        this.audioOptions = source["audioOptions"];
	        this.command = source["command"];
	        this.actions = this.convertValues(source["actions"], TaskActions);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class Preferences {
	    workDir: string;
	    selectPage: string;
	    videoIndex: string;
	    audioIndex: string;
	    dfnPriority: string;
	    encodingPriority: string;
	    extraArgs: string;
	    ffmpegPath: string;
	    mp4boxPath: string;
	    aria2cPath: string;
	    fileExistsAction: string;
	    mode: string;
	    channel: string;
	    downloadDanmaku: boolean;
	    skipSubtitle: boolean;
	    skipCover: boolean;
	    skipMux: boolean;
	    useAria2c: boolean;
	    autoQueue: boolean;
	    theme: string;
	
	    static createFrom(source: any = {}) {
	        return new Preferences(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.workDir = source["workDir"];
	        this.selectPage = source["selectPage"];
	        this.videoIndex = source["videoIndex"];
	        this.audioIndex = source["audioIndex"];
	        this.dfnPriority = source["dfnPriority"];
	        this.encodingPriority = source["encodingPriority"];
	        this.extraArgs = source["extraArgs"];
	        this.ffmpegPath = source["ffmpegPath"];
	        this.mp4boxPath = source["mp4boxPath"];
	        this.aria2cPath = source["aria2cPath"];
	        this.fileExistsAction = source["fileExistsAction"];
	        this.mode = source["mode"];
	        this.channel = source["channel"];
	        this.downloadDanmaku = source["downloadDanmaku"];
	        this.skipSubtitle = source["skipSubtitle"];
	        this.skipCover = source["skipCover"];
	        this.skipMux = source["skipMux"];
	        this.useAria2c = source["useAria2c"];
	        this.autoQueue = source["autoQueue"];
	        this.theme = source["theme"];
	    }
	}
	export class Bootstrap {
	    preferences: Preferences;
	    tasks: TaskDTO[];
	    summary: QueueSummary;
	    version: string;
	    buildTime: string;
	    status: string;
	
	    static createFrom(source: any = {}) {
	        return new Bootstrap(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.preferences = this.convertValues(source["preferences"], Preferences);
	        this.tasks = this.convertValues(source["tasks"], TaskDTO);
	        this.summary = this.convertValues(source["summary"], QueueSummary);
	        this.version = source["version"];
	        this.buildTime = source["buildTime"];
	        this.status = source["status"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class LoginSessionDTO {
	    id: number;
	    kind: string;
	    status: string;
	    message: string;
	    log: string;
	    running: boolean;
	    successful: boolean;
	
	    static createFrom(source: any = {}) {
	        return new LoginSessionDTO(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.kind = source["kind"];
	        this.status = source["status"];
	        this.message = source["message"];
	        this.log = source["log"];
	        this.running = source["running"];
	        this.successful = source["successful"];
	    }
	}
	
	
	
	
	
	export class TaskInput {
	    url: string;
	    workDir: string;
	    selectPage: string;
	    videoIndex: string;
	    audioIndex: string;
	    dfnPriority: string;
	    encodingPriority: string;
	    extraArgs: string;
	    ffmpegPath: string;
	    mp4boxPath: string;
	    aria2cPath: string;
	    fileExistsAction: string;
	    mode: string;
	    channel: string;
	    downloadDanmaku: boolean;
	    skipSubtitle: boolean;
	    skipCover: boolean;
	    skipMux: boolean;
	    useAria2c: boolean;
	    autoQueue: boolean;
	    theme: string;
	
	    static createFrom(source: any = {}) {
	        return new TaskInput(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.url = source["url"];
	        this.workDir = source["workDir"];
	        this.selectPage = source["selectPage"];
	        this.videoIndex = source["videoIndex"];
	        this.audioIndex = source["audioIndex"];
	        this.dfnPriority = source["dfnPriority"];
	        this.encodingPriority = source["encodingPriority"];
	        this.extraArgs = source["extraArgs"];
	        this.ffmpegPath = source["ffmpegPath"];
	        this.mp4boxPath = source["mp4boxPath"];
	        this.aria2cPath = source["aria2cPath"];
	        this.fileExistsAction = source["fileExistsAction"];
	        this.mode = source["mode"];
	        this.channel = source["channel"];
	        this.downloadDanmaku = source["downloadDanmaku"];
	        this.skipSubtitle = source["skipSubtitle"];
	        this.skipCover = source["skipCover"];
	        this.skipMux = source["skipMux"];
	        this.useAria2c = source["useAria2c"];
	        this.autoQueue = source["autoQueue"];
	        this.theme = source["theme"];
	    }
	}
	export class ToolDiagnostic {
	    name: string;
	    found: boolean;
	    path: string;
	    version: string;
	    message: string;
	    installHint: string;
	
	    static createFrom(source: any = {}) {
	        return new ToolDiagnostic(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.found = source["found"];
	        this.path = source["path"];
	        this.version = source["version"];
	        this.message = source["message"];
	        this.installHint = source["installHint"];
	    }
	}
	export class ToolPaths {
	    ffmpegPath: string;
	    mp4boxPath: string;
	    aria2cPath: string;
	
	    static createFrom(source: any = {}) {
	        return new ToolPaths(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.ffmpegPath = source["ffmpegPath"];
	        this.mp4boxPath = source["mp4boxPath"];
	        this.aria2cPath = source["aria2cPath"];
	    }
	}
	export class VersionInfo {
	    version: string;
	    buildTime: string;
	
	    static createFrom(source: any = {}) {
	        return new VersionInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.version = source["version"];
	        this.buildTime = source["buildTime"];
	    }
	}

}

