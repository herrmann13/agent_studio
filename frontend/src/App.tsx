import {Component, ErrorInfo, FormEvent, ReactNode, useEffect, useState} from 'react';
import {AddProject, CheckForUpdates, CopySkill, DeleteSkill, DiscoverLocalEnvironment, DownloadAndInstallUpdate, InstallSkillFromURL, RemoveProject, SelectProjectFolder} from '../wailsjs/go/main/App';
import {domain, update} from '../wailsjs/go/models';
import './App.css';

function App() {
    const [workspace, setWorkspace] = useState<domain.DiscoveryResult>();
    const [selectedProjectPath, setSelectedProjectPath] = useState('');
    const [draggedSkill, setDraggedSkill] = useState<domain.Skill>();
    const [contextMenu, setContextMenu] = useState<{skill: domain.Skill; x: number; y: number}>();
    const [addSkillTarget, setAddSkillTarget] = useState<domain.Scope>();
    const [skillSearch, setSkillSearch] = useState('');
    const [message, setMessage] = useState<string>();
    const [isLoading, setIsLoading] = useState(true);
    const [skillURL, setSkillURL] = useState('');
    const [skillTargetID, setSkillTargetID] = useState('global');
    const [isInstalling, setIsInstalling] = useState(false);
    const [isInstallDialogOpen, setIsInstallDialogOpen] = useState(false);
    const [availableUpdate, setAvailableUpdate] = useState<update.Info>();
    const [isCheckingForUpdates, setIsCheckingForUpdates] = useState(false);
    const [isUpdating, setIsUpdating] = useState(false);

    async function refresh() {
        setIsLoading(true);
        try {
            setWorkspace(normalizeWorkspace(await DiscoverLocalEnvironment()));
            setMessage(undefined);
        } catch (error) {
            setMessage(error instanceof Error ? error.message : 'Could not inspect the local workspace.');
        } finally {
            setIsLoading(false);
        }
    }

    useEffect(() => { void refresh(); }, []);

    useEffect(() => {
        if (!isInstallDialogOpen) return;
        const closeOnEscape = (event: KeyboardEvent) => {
            if (event.key === 'Escape' && !isInstalling) setIsInstallDialogOpen(false);
        };
        window.addEventListener('keydown', closeOnEscape);
        return () => window.removeEventListener('keydown', closeOnEscape);
    }, [isInstallDialogOpen, isInstalling]);

    async function selectProject() {
        try {
            const path = await SelectProjectFolder();
            if (!path) return;
            setSelectedProjectPath(path);
            setWorkspace(normalizeWorkspace(await AddProject(path)));
            setMessage(undefined);
        } catch (error) {
            setMessage(error instanceof Error ? error.message : 'Could not select project.');
        }
    }

    async function dropSkill(scope: domain.Scope) {
        if (!draggedSkill || draggedSkill.scopeId === scope.id) return;
        try {
            setWorkspace(normalizeWorkspace(await CopySkill(draggedSkill.path, scope.id)));
            setMessage(`Copied ${draggedSkill.name} to ${scope.name}.`);
        } catch (error) {
            setMessage(error instanceof Error ? error.message : 'Could not copy skill.');
        } finally {
            setDraggedSkill(undefined);
        }
    }

    async function deleteSkill(skill: domain.Skill) {
        setContextMenu(undefined);
        if (!window.confirm(`Delete "${skill.name}" from this location? A backup will be created first.`)) return;
        try {
            setWorkspace(normalizeWorkspace(await DeleteSkill(skill.path)));
            setMessage(`Deleted ${skill.name}. A backup was created locally.`);
        } catch (error) {
            setMessage(error instanceof Error ? error.message : 'Could not delete skill.');
        }
    }

    function openContextMenu(skill: domain.Skill, x: number, y: number) {
        const menuWidth = 192;
        const menuHeight = 88;
        setContextMenu({
            skill,
            x: Math.max(8, Math.min(x, window.innerWidth - menuWidth - 8)),
            y: Math.max(8, Math.min(y, window.innerHeight - menuHeight - 8)),
        });
    }

    async function removeProject(project: domain.Project) {
        if (!window.confirm(`Stop tracking "${project.name}"? The project and its skills will not be deleted.`)) return;
        try {
            setWorkspace(normalizeWorkspace(await RemoveProject(project.id)));
            setMessage(`${project.name} is no longer tracked.`);
        } catch (error) {
            setMessage(error instanceof Error ? error.message : 'Could not stop tracking project.');
        }
    }

    async function addSkillToProject(skill: domain.Skill, target: domain.Scope) {
        try {
            setWorkspace(normalizeWorkspace(await CopySkill(skill.path, target.id)));
            setMessage(`${skill.name} was copied to ${target.name}.`);
        } catch (error) {
            setMessage(error instanceof Error ? error.message : 'Could not add skill to project.');
        }
    }

    async function installFromURL(event: FormEvent<HTMLFormElement>) {
        event.preventDefault();
        if (!skillURL.trim() || !skillTargetID) return;
        setIsInstalling(true);
        try {
            const result = await InstallSkillFromURL(skillURL.trim(), skillTargetID);
            setWorkspace(normalizeWorkspace(result.workspace));
            setMessage(`Skill installed via ${result.method} and is now available in the selected destination.`);
            setSkillURL('');
        } catch (error) {
            setMessage(error instanceof Error ? error.message : 'Could not install skill from URL.');
        } finally {
            setIsInstalling(false);
        }
    }

    async function checkForUpdates() {
        setIsCheckingForUpdates(true);
        try {
            const info = await CheckForUpdates();
            if (!info.updateAvailable) {
                setMessage(`Agent Studio ${info.currentVersion} is up to date.`);
                return;
            }
            setAvailableUpdate(info);
        } catch (error) {
            setMessage(error instanceof Error ? error.message : 'Could not check for updates.');
        } finally {
            setIsCheckingForUpdates(false);
        }
    }

    async function installUpdate() {
        if (!availableUpdate) return;
        setIsUpdating(true);
        try {
            await DownloadAndInstallUpdate(availableUpdate.latestVersion);
            setAvailableUpdate(undefined);
            setMessage('Update installed. Restart Agent Studio to use the new version.');
        } catch (error) {
            setMessage(error instanceof Error ? error.message : 'Could not install the update.');
        } finally {
            setIsUpdating(false);
        }
    }

    const scopes = workspace?.scopes ?? [];
    const globalScope = scopes.find((scope) => scope.kind === 'global');
    const agentScopes = scopes.filter((scope) => scope.kind === 'agent');
    const projectScopes = scopes.filter((scope) => scope.kind === 'project');

    return (
        <main className="studio-shell" onClick={() => contextMenu && setContextMenu(undefined)}>
            <header className="studio-header">
                <div>
                    <p className="eyebrow">SKILL WORKSPACE</p>
                    <h1>Agent Studio</h1>
                    <p className="intro">Copy skills between global, agent, and project locations. Nothing moves or deletes implicitly.</p>
                </div>
                <div className="header-actions"><button className="install-button" type="button" onClick={() => setIsInstallDialogOpen(true)}>Install skill</button><button className="refresh-button" type="button" onClick={() => void checkForUpdates()} disabled={isCheckingForUpdates}>{isCheckingForUpdates ? 'Checking...' : 'Check updates'}</button><button className="refresh-button" type="button" onClick={() => void refresh()} disabled={isLoading}>{isLoading ? 'Scanning...' : 'Refresh'}</button></div>
            </header>

            <section className="workspace-summary">
                <Metric value={workspace?.skills.length ?? 0} label="Skill locations"/>
                <Metric value={workspace?.projects.length ?? 0} label="Tracked projects"/>
                <Metric value={workspace?.skills.filter((skill) => skill.states.includes('conflict')).length ?? 0} label="Conflicts"/>
                <p className="read-only-note">Filesystem writes only happen when you drop a skill into a destination or confirm Delete.</p>
            </section>

            <section className="project-form">
                <div><label>Track a project</label><span>Select a folder to scan its project skills.</span></div>
                <button type="button" onClick={() => void selectProject()}>Select project folder</button>
                {selectedProjectPath ? <div className="selected-project"><span>Selected project</span><code title={selectedProjectPath}>{selectedProjectPath}</code></div> : null}
            </section>

            {message ? <p className="workspace-message">{message}</p> : null}

            <section className="workspace-section">
                <SectionHeading label="BASE LAYERS" title="Global and agents"/>
                <div className="scope-grid">
                    {globalScope ? <ScopeLane scope={globalScope} skills={skillsInScope(workspace, globalScope.id)} draggedSkill={draggedSkill} onDragStart={setDraggedSkill} onDrop={dropSkill} onContextMenu={openContextMenu}/> : null}
                    {agentScopes.map((scope) => <ScopeLane key={scope.id} scope={scope} skills={skillsInScope(workspace, scope.id)} draggedSkill={draggedSkill} onDragStart={setDraggedSkill} onDrop={dropSkill} onContextMenu={openContextMenu}/>) }
                </div>
            </section>

            <section className="workspace-section">
                <SectionHeading label="PROJECT LAYERS" title="Selected projects"/>
                {projectScopes.length ? <div className="scope-grid project-grid">
                    {projectScopes.map((scope) => {
                        const project = workspace?.projects.find((item) => item.id === scope.id.replace('project:', ''));
                        return <ScopeLane key={scope.id} scope={scope} project={project} skills={skillsInScope(workspace, scope.id)} draggedSkill={draggedSkill} onDragStart={setDraggedSkill} onDrop={dropSkill} onContextMenu={openContextMenu} onRemoveProject={project ? () => void removeProject(project) : undefined} onAddSkill={() => { setAddSkillTarget(scope); setSkillSearch(''); }}/>;
                    })}
                </div> : <p className="empty-projects">Add a project path to create its `.agents/skills` destination.</p>}
            </section>

            {contextMenu ? <div className="context-menu" role="menu" style={{left: contextMenu.x, top: contextMenu.y}} onClick={(event) => event.stopPropagation()}>
                <button type="button" role="menuitem" onClick={() => void deleteSkill(contextMenu.skill)}>Delete skill</button>
                <strong>{contextMenu.skill.name}</strong>
            </div> : null}

            {addSkillTarget ? <AddSkillModal
                target={addSkillTarget}
                skills={workspace?.skills ?? []}
                search={skillSearch}
                onSearch={setSkillSearch}
                onAdd={(skill) => void addSkillToProject(skill, addSkillTarget)}
                onClose={() => setAddSkillTarget(undefined)}
            /> : null}
            {isInstallDialogOpen ? <InstallSkillDialog url={skillURL} targetID={skillTargetID} scopes={scopes} isInstalling={isInstalling} onURLChange={setSkillURL} onTargetChange={setSkillTargetID} onSubmit={installFromURL} onClose={() => { if (!isInstalling) setIsInstallDialogOpen(false); }}/> : null}
            {availableUpdate ? <UpdateDialog update={availableUpdate} isUpdating={isUpdating} onInstall={() => void installUpdate()} onClose={() => { if (!isUpdating) setAvailableUpdate(undefined); }}/> : null}
        </main>
    );
}

function ScopeLane({scope, project, skills, draggedSkill, onDragStart, onDrop, onContextMenu, onRemoveProject, onAddSkill}: {
    scope: domain.Scope;
    project?: domain.Project;
    skills: domain.Skill[];
    draggedSkill: domain.Skill | undefined;
    onDragStart: (skill: domain.Skill) => void;
    onDrop: (scope: domain.Scope) => void;
    onContextMenu: (skill: domain.Skill, x: number, y: number) => void;
    onRemoveProject?: () => void;
    onAddSkill?: () => void;
}) {
    const [isDropTarget, setIsDropTarget] = useState(false);
    const [expandedSkills, setExpandedSkills] = useState<Set<string>>(new Set());
    const [projectMenuOpen, setProjectMenuOpen] = useState(false);

    function toggleSkill(skillID: string) {
        setExpandedSkills((current) => {
            const next = new Set(current);
            if (next.has(skillID)) next.delete(skillID);
            else next.add(skillID);
            return next;
        });
    }

    return <section
        className={`scope-lane ${isDropTarget ? 'is-drop-target' : ''}`}
        onDragOver={(event) => { event.preventDefault(); setIsDropTarget(Boolean(draggedSkill && draggedSkill.scopeId !== scope.id)); }}
        onDragLeave={() => setIsDropTarget(false)}
        onDrop={(event) => { event.preventDefault(); setIsDropTarget(false); onDrop(scope); }}
    >
        <div className="scope-heading">
            <div><span className={`scope-icon ${scope.kind}`}>{scope.kind === 'global' ? 'G' : scope.kind === 'agent' ? scope.name.slice(0, 1) : 'P'}</span><strong>{scope.name}</strong></div>
            <div className="scope-actions">
                <span>{skills.length}</span>
                {scope.kind === 'project' && onRemoveProject ? <div className="project-menu-wrap">
                    <button className="project-menu-button" type="button" aria-label={`Project menu for ${scope.name}`} aria-expanded={projectMenuOpen} onClick={(event) => { event.stopPropagation(); setProjectMenuOpen((open) => !open); }}>⋮</button>
                    {projectMenuOpen ? <div className="project-menu" role="menu" onClick={(event) => event.stopPropagation()}><button type="button" role="menuitem" onClick={() => { setProjectMenuOpen(false); onAddSkill?.(); }}>Add skill</button><button type="button" role="menuitem" onClick={() => { setProjectMenuOpen(false); onRemoveProject(); }}>Stop tracking</button></div> : null}
                </div> : null}
            </div>
        </div>
        <p className="scope-kind">{scope.kind === 'project' ? 'Project skills' : scope.kind === 'global' ? 'Shared skills' : 'Agent skills'}</p>
        {project ? <code className="project-path" title={project.path}>{project.path}</code> : null}
        <div className="skill-stack">
            {skills.map((skill) => {
                const isExpanded = expandedSkills.has(skill.id);
                return <article className={`skill-card ${isExpanded ? 'is-expanded' : ''}`} key={skill.id} draggable onDragStart={() => onDragStart(skill)} onDragEnd={() => setIsDropTarget(false)} onContextMenu={(event) => { event.preventDefault(); onContextMenu(skill, event.clientX, event.clientY); }}>
                    <button className="skill-card-toggle" type="button" aria-expanded={isExpanded} onClick={() => toggleSkill(skill.id)}>
                        <strong>{skill.name}</strong>
                        <span className="skill-card-action"><span className="skill-state-count">{skill.states?.length ?? 0}</span><span className="expand-icon" aria-hidden="true">{isExpanded ? '−' : '+'}</span></span>
                    </button>
                    {isExpanded ? <div className="skill-card-details">
                        <p>{skill.description}</p>
                        <small>{skill.path}</small>
                        <div className="skill-states">{(skill.states ?? []).map((state) => <span className={state} key={state}>{state}</span>)}</div>
                    </div> : null}
                    <span className="drag-handle" aria-label="Drag to copy">::</span>
                </article>;
            })}
            {!skills.length ? <p className="lane-placeholder">Drop a skill here to copy it.</p> : null}
        </div>
    </section>;
}

function SectionHeading({label, title}: {label: string; title: string}) {
    return <div className="section-heading"><p>{label}</p><h2>{title}</h2></div>;
}

function Metric({value, label}: {value: number; label: string}) {
    return <div className="metric"><strong>{value}</strong><span>{label}</span></div>;
}

function AddSkillModal({target, skills, search, onSearch, onAdd, onClose}: {
    target: domain.Scope;
    skills: domain.Skill[];
    search: string;
    onSearch: (value: string) => void;
    onAdd: (skill: domain.Skill) => void;
    onClose: () => void;
}) {
    const normalizedSearch = search.trim().toLowerCase();
    const targetNames = new Set(skills.filter((skill) => skill.scopeId === target.id).map((skill) => skill.name.toLowerCase()));
    const availableSkills = skills.filter((skill) => !normalizedSearch || `${skill.name} ${skill.description}`.toLowerCase().includes(normalizedSearch));

    return <div className="modal-backdrop" role="presentation" onClick={onClose}>
        <section className="skill-modal" role="dialog" aria-modal="true" aria-labelledby="add-skill-title" onClick={(event) => event.stopPropagation()}>
            <div className="modal-heading"><div><p className="section-kicker">PROJECT SKILLS</p><h2 id="add-skill-title">Add skill to {target.name}</h2></div><button className="modal-close" type="button" aria-label="Close" onClick={onClose}>×</button></div>
            <input className="skill-search" value={search} onChange={(event) => onSearch(event.target.value)} placeholder="Search skills by name or description" autoFocus/>
            <div className="modal-skill-list">
                {availableSkills.map((skill) => {
                    const alreadyAdded = targetNames.has(skill.name.toLowerCase());
                    return <article className="modal-skill-row" key={`${skill.id}-${skill.scopeId}`}><div><strong>{skill.name}</strong><p>{skill.description}</p><small>{skill.scopeId}</small></div><button type="button" disabled={alreadyAdded} onClick={() => onAdd(skill)}>{alreadyAdded ? 'Added' : 'Add'}</button></article>;
                })}
                {!availableSkills.length ? <p className="empty-state">No skills match your search.</p> : null}
            </div>
        </section>
    </div>;
}

function InstallSkillDialog({url, targetID, scopes, isInstalling, onURLChange, onTargetChange, onSubmit, onClose}: {
    url: string;
    targetID: string;
    scopes: domain.Scope[];
    isInstalling: boolean;
    onURLChange: (value: string) => void;
    onTargetChange: (value: string) => void;
    onSubmit: (event: FormEvent<HTMLFormElement>) => void;
    onClose: () => void;
}) {
    return <div className="modal-backdrop" role="presentation" onClick={onClose}>
        <section className="install-modal" role="dialog" aria-modal="true" aria-labelledby="install-skill-title" onClick={(event) => event.stopPropagation()}>
            <div className="modal-heading"><div><p className="section-kicker">PUBLIC REPOSITORY</p><h2 id="install-skill-title">Install skill</h2></div><button className="modal-close" type="button" aria-label="Close" onClick={onClose} disabled={isInstalling}>×</button></div>
            <p className="install-help">Paste a public GitHub, GitLab, or Bitbucket repository URL. Agent Studio will use Git when available and ZIP otherwise.</p>
            <form className="install-dialog-form" onSubmit={onSubmit}>
                <label htmlFor="skill-url">Repository or skill folder URL</label>
                <input id="skill-url" value={url} onChange={(event) => onURLChange(event.target.value)} placeholder="https://github.com/owner/repository/tree/main/skills/my-skill" autoFocus required/>
                <label htmlFor="skill-target">Install into</label>
                <select id="skill-target" value={targetID} onChange={(event) => onTargetChange(event.target.value)} required>{scopes.map((scope) => <option key={scope.id} value={scope.id}>{scope.name}</option>)}</select>
                <div className="install-dialog-actions"><button type="button" className="modal-secondary" onClick={onClose} disabled={isInstalling}>Cancel</button><button type="submit" className="install-submit" disabled={isInstalling}>{isInstalling ? 'Installing...' : 'Install skill'}</button></div>
            </form>
        </section>
    </div>;
}

function UpdateDialog({update, isUpdating, onInstall, onClose}: {
    update: update.Info;
    isUpdating: boolean;
    onInstall: () => void;
    onClose: () => void;
}) {
    return <div className="modal-backdrop" role="presentation" onClick={onClose}>
        <section className="install-modal" role="dialog" aria-modal="true" aria-labelledby="update-title" onClick={(event) => event.stopPropagation()}>
            <div className="modal-heading"><div><p className="section-kicker">UPDATE AVAILABLE</p><h2 id="update-title">{update.latestVersion}</h2></div><button className="modal-close" type="button" aria-label="Close" onClick={onClose} disabled={isUpdating}>×</button></div>
            <p className="install-help">You are using {update.currentVersion}. The installer will be downloaded, verified, and installed automatically.</p>
            {update.releaseNotes ? <pre className="update-notes">{update.releaseNotes}</pre> : null}
            <div className="install-dialog-actions"><button type="button" className="modal-secondary" onClick={onClose} disabled={isUpdating}>Later</button><button type="button" className="install-submit" onClick={onInstall} disabled={isUpdating}>{isUpdating ? 'Installing...' : 'Download and install'}</button></div>
        </section>
    </div>;
}

function skillsInScope(workspace: domain.DiscoveryResult | undefined, scopeID: string) {
    return workspace?.skills.filter((skill) => skill.scopeId === scopeID) ?? [];
}

function normalizeWorkspace(workspace: domain.DiscoveryResult): domain.DiscoveryResult {
    workspace.agents ??= [];
    workspace.skills ??= [];
    workspace.configFiles ??= [];
    workspace.scopes ??= [];
    workspace.projects ??= [];
    return workspace;
}

class ErrorBoundary extends Component<{children: ReactNode}, {error?: Error}> {
    state: {error?: Error} = {};

    static getDerivedStateFromError(error: Error) {
        return {error};
    }

    componentDidCatch(error: Error, info: ErrorInfo) {
        console.error('Agent Studio failed to render', error, info.componentStack);
    }

    render() {
        if (!this.state.error) return this.props.children;
        return <main className="fatal-error"><p className="eyebrow">AGENT STUDIO</p><h1>Could not render the workspace</h1><p>{this.state.error.message}</p><button onClick={() => window.location.reload()}>Reload</button></main>;
    }
}

export default function RootApp() {
    return <ErrorBoundary><App /></ErrorBoundary>;
}
