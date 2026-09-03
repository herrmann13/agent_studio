import {FormEvent, useEffect, useState} from 'react';
import {AddProject, CopySkill, DeleteSkill, DiscoverLocalEnvironment} from '../wailsjs/go/main/App';
import {domain} from '../wailsjs/go/models';
import './App.css';

function App() {
    const [workspace, setWorkspace] = useState<domain.DiscoveryResult>();
    const [projectPath, setProjectPath] = useState('');
    const [draggedSkill, setDraggedSkill] = useState<domain.Skill>();
    const [contextSkill, setContextSkill] = useState<domain.Skill>();
    const [message, setMessage] = useState<string>();
    const [isLoading, setIsLoading] = useState(true);

    async function refresh() {
        setIsLoading(true);
        try {
            setWorkspace(await DiscoverLocalEnvironment());
            setMessage(undefined);
        } catch (error) {
            setMessage(error instanceof Error ? error.message : 'Could not inspect the local workspace.');
        } finally {
            setIsLoading(false);
        }
    }

    useEffect(() => { void refresh(); }, []);

    async function addProject(event: FormEvent<HTMLFormElement>) {
        event.preventDefault();
        if (!projectPath.trim()) return;
        try {
            setWorkspace(await AddProject(projectPath.trim()));
            setProjectPath('');
            setMessage(undefined);
        } catch (error) {
            setMessage(error instanceof Error ? error.message : 'Could not add project.');
        }
    }

    async function dropSkill(scope: domain.Scope) {
        if (!draggedSkill || draggedSkill.scopeId === scope.id) return;
        try {
            setWorkspace(await CopySkill(draggedSkill.path, scope.id));
            setMessage(`Copied ${draggedSkill.name} to ${scope.name}.`);
        } catch (error) {
            setMessage(error instanceof Error ? error.message : 'Could not copy skill.');
        } finally {
            setDraggedSkill(undefined);
        }
    }

    async function deleteSkill(skill: domain.Skill) {
        setContextSkill(undefined);
        if (!window.confirm(`Delete "${skill.name}" from this location? A backup will be created first.`)) return;
        try {
            setWorkspace(await DeleteSkill(skill.path));
            setMessage(`Deleted ${skill.name}. A backup was created locally.`);
        } catch (error) {
            setMessage(error instanceof Error ? error.message : 'Could not delete skill.');
        }
    }

    const scopes = workspace?.scopes ?? [];
    const globalScope = scopes.find((scope) => scope.kind === 'global');
    const agentScopes = scopes.filter((scope) => scope.kind === 'agent');
    const projectScopes = scopes.filter((scope) => scope.kind === 'project');

    return (
        <main className="studio-shell" onClick={() => contextSkill && setContextSkill(undefined)}>
            <header className="studio-header">
                <div>
                    <p className="eyebrow">SKILL WORKSPACE</p>
                    <h1>Agent Studio</h1>
                    <p className="intro">Copy skills between global, agent, and project locations. Nothing moves or deletes implicitly.</p>
                </div>
                <button className="refresh-button" onClick={() => void refresh()} disabled={isLoading}>{isLoading ? 'Scanning...' : 'Refresh'}</button>
            </header>

            <section className="workspace-summary">
                <Metric value={workspace?.skills.length ?? 0} label="Skill locations"/>
                <Metric value={workspace?.projects.length ?? 0} label="Tracked projects"/>
                <Metric value={workspace?.skills.filter((skill) => skill.states.includes('conflict')).length ?? 0} label="Conflicts"/>
                <p className="read-only-note">Filesystem writes only happen when you drop a skill into a destination or confirm Delete.</p>
            </section>

            <form className="project-form" onSubmit={addProject}>
                <label htmlFor="project-path">Track a project</label>
                <input id="project-path" value={projectPath} onChange={(event) => setProjectPath(event.target.value)} placeholder="/path/to/project"/>
                <button type="submit">Add project</button>
            </form>

            {message ? <p className="workspace-message">{message}</p> : null}

            <section className="workspace-section">
                <SectionHeading label="BASE LAYERS" title="Global and agents"/>
                <div className="scope-grid">
                    {globalScope ? <ScopeLane scope={globalScope} skills={skillsInScope(workspace, globalScope.id)} draggedSkill={draggedSkill} onDragStart={setDraggedSkill} onDrop={dropSkill} onContextSkill={setContextSkill}/> : null}
                    {agentScopes.map((scope) => <ScopeLane key={scope.id} scope={scope} skills={skillsInScope(workspace, scope.id)} draggedSkill={draggedSkill} onDragStart={setDraggedSkill} onDrop={dropSkill} onContextSkill={setContextSkill}/>) }
                </div>
            </section>

            <section className="workspace-section">
                <SectionHeading label="PROJECT LAYERS" title="Selected projects"/>
                {projectScopes.length ? <div className="scope-grid project-grid">
                    {projectScopes.map((scope) => <ScopeLane key={scope.id} scope={scope} skills={skillsInScope(workspace, scope.id)} draggedSkill={draggedSkill} onDragStart={setDraggedSkill} onDrop={dropSkill} onContextSkill={setContextSkill}/>) }
                </div> : <p className="empty-projects">Add a project path to create its `.agents/skills` destination.</p>}
            </section>

            {contextSkill ? <div className="context-menu" role="menu" onClick={(event) => event.stopPropagation()}>
                <strong>{contextSkill.name}</strong>
                <button role="menuitem" onClick={() => void deleteSkill(contextSkill)}>Delete skill</button>
            </div> : null}
        </main>
    );
}

function ScopeLane({scope, skills, draggedSkill, onDragStart, onDrop, onContextSkill}: {
    scope: domain.Scope;
    skills: domain.Skill[];
    draggedSkill: domain.Skill | undefined;
    onDragStart: (skill: domain.Skill) => void;
    onDrop: (scope: domain.Scope) => void;
    onContextSkill: (skill: domain.Skill) => void;
}) {
    const [isDropTarget, setIsDropTarget] = useState(false);
    return <section
        className={`scope-lane ${isDropTarget ? 'is-drop-target' : ''}`}
        onDragOver={(event) => { event.preventDefault(); setIsDropTarget(Boolean(draggedSkill && draggedSkill.scopeId !== scope.id)); }}
        onDragLeave={() => setIsDropTarget(false)}
        onDrop={(event) => { event.preventDefault(); setIsDropTarget(false); onDrop(scope); }}
    >
        <div className="scope-heading">
            <div><span className={`scope-icon ${scope.kind}`}>{scope.kind === 'global' ? 'G' : scope.kind === 'agent' ? scope.name.slice(0, 1) : 'P'}</span><strong>{scope.name}</strong></div>
            <span>{skills.length}</span>
        </div>
        <p className="scope-kind">{scope.kind === 'project' ? 'Project skills' : scope.kind === 'global' ? 'Shared skills' : 'Agent skills'}</p>
        <div className="skill-stack">
            {skills.map((skill) => <article className="skill-card" key={skill.id} draggable onDragStart={() => onDragStart(skill)} onDragEnd={() => setIsDropTarget(false)} onContextMenu={(event) => { event.preventDefault(); onContextSkill(skill); }}>
                <div className="skill-card-heading"><strong>{skill.name}</strong><span className="drag-handle" aria-label="Drag to copy">::</span></div>
                <p>{skill.description}</p>
                <div className="skill-states">{skill.states.map((state) => <span className={state} key={state}>{state}</span>)}</div>
            </article>)}
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

function skillsInScope(workspace: domain.DiscoveryResult | undefined, scopeID: string) {
    return workspace?.skills.filter((skill) => skill.scopeId === scopeID) ?? [];
}

export default App;
