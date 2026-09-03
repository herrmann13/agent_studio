export namespace domain {
	
	export class Agent {
	    id: string;
	    name: string;
	    provider: string;
	    status: string;
	    configPath: string;
	    commandPath?: string;
	
	    static createFrom(source: any = {}) {
	        return new Agent(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.name = source["name"];
	        this.provider = source["provider"];
	        this.status = source["status"];
	        this.configPath = source["configPath"];
	        this.commandPath = source["commandPath"];
	    }
	}
	export class ConfigFile {
	    provider: string;
	    path: string;
	    scope: string;
	
	    static createFrom(source: any = {}) {
	        return new ConfigFile(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.provider = source["provider"];
	        this.path = source["path"];
	        this.scope = source["scope"];
	    }
	}
	export class Project {
	    id: string;
	    name: string;
	    path: string;
	
	    static createFrom(source: any = {}) {
	        return new Project(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.name = source["name"];
	        this.path = source["path"];
	    }
	}
	export class Scope {
	    id: string;
	    name: string;
	    kind: string;
	    provider?: string;
	    root: string;
	
	    static createFrom(source: any = {}) {
	        return new Scope(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.name = source["name"];
	        this.kind = source["kind"];
	        this.provider = source["provider"];
	        this.root = source["root"];
	    }
	}
	export class Skill {
	    id: string;
	    name: string;
	    description: string;
	    path: string;
	    scopeId: string;
	    states: string[];
	    contentHash: string;
	
	    static createFrom(source: any = {}) {
	        return new Skill(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.name = source["name"];
	        this.description = source["description"];
	        this.path = source["path"];
	        this.scopeId = source["scopeId"];
	        this.states = source["states"];
	        this.contentHash = source["contentHash"];
	    }
	}
	export class DiscoveryResult {
	    agents: Agent[];
	    skills: Skill[];
	    configFiles: ConfigFile[];
	    scopes: Scope[];
	    projects: Project[];
	    scannedAt: string;
	
	    static createFrom(source: any = {}) {
	        return new DiscoveryResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.agents = this.convertValues(source["agents"], Agent);
	        this.skills = this.convertValues(source["skills"], Skill);
	        this.configFiles = this.convertValues(source["configFiles"], ConfigFile);
	        this.scopes = this.convertValues(source["scopes"], Scope);
	        this.projects = this.convertValues(source["projects"], Project);
	        this.scannedAt = source["scannedAt"];
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
	
	

}

