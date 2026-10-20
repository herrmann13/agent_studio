import {Component, ErrorInfo, FormEvent, ReactNode, useEffect, useRef, useState} from 'react';
import {AddProject, CheckForUpdates, CopySkill, DeleteSkill, DiscoverLocalEnvironment, DownloadAndInstallUpdate, InstallRegistrySkill, InstallSkillFromURL, RemoveProject, ReorderProjects, SearchRegistrySkills, SelectProjectFolder, SetAgentOrder, SetSkillInvocationMode, SetTrackedAgents, SyncGlobalSkills} from '../wailsjs/go/main/App';
import {domain, update} from '../wailsjs/go/models';
import {BrowserOpenURL} from '../wailsjs/runtime/runtime';
import claudeIcon from './assets/agents/Claude_AI_symbol.svg.webp';
import codexIcon from './assets/agents/Codex Logo - Colored - zonalogo.com.svg';
import openCodeIcon from './assets/agents/opencode-logo-light.svg';
import './App.css';

const usePointerDrag = typeof navigator !== 'undefined' && /Linux/i.test(navigator.platform || navigator.userAgent);

type LaneGroup = 'agents' | 'projects';

// LaneReorder lets a lane be dragged by its grip onto another lane of the same
// group. It uses pointer events on every platform, so it never competes with the
// HTML5 drag that copies skills between lanes.
type LaneReorder = {
    group: LaneGroup;
    draggingID?: string;
    overID?: string;
    onStart: () => void;
    onOver: (scopeID: string | undefined) => void;
    onDrop: (targetID: string | undefined) => void;
    onStep: (offset: number) => void;
};

function App() {
    const [workspace, setWorkspace] = useState<domain.DiscoveryResult>();
    const [selectedProjectPath, setSelectedProjectPath] = useState('');
    const [draggedSkill, setDraggedSkill] = useState<domain.Skill>();
    const [contextMenu, setContextMenu] = useState<{skill: domain.Skill; x: number; y: number}>();
    const [pendingDeletion, setPendingDeletion] = useState<domain.Skill>();
    const [isDeleting, setIsDeleting] = useState(false);
    const [addSkillTarget, setAddSkillTarget] = useState<domain.Scope>();
    const [skillSearch, setSkillSearch] = useState('');
    const [message, setMessage] = useState<string>();
    const [isLoading, setIsLoading] = useState(true);
    const [skillURL, setSkillURL] = useState('');
    const [skillTargetID, setSkillTargetID] = useState('');
    const [isInstalling, setIsInstalling] = useState(false);
    const [isInstallDialogOpen, setIsInstallDialogOpen] = useState(false);
    const [availableUpdate, setAvailableUpdate] = useState<update.Info>();
    const [isCheckingForUpdates, setIsCheckingForUpdates] = useState(false);
    const [isUpdating, setIsUpdating] = useState(false);
    const [pointerDropScopeID, setPointerDropScopeID] = useState<string>();
    const [laneDrag, setLaneDrag] = useState<{group: LaneGroup; id: string; overID?: string}>();
    const [isAgentsDialogOpen, setIsAgentsDialogOpen] = useState(false);
    // Lane order saves run one at a time, and only the newest reply is applied,
    // so quick reorders can neither persist an older order last nor make a lane
    // jump back when an older reply arrives.
    const laneSaveQueue = useRef<Promise<void>>(Promise.resolve());
    const laneSaveSequence = useRef(0);

    async function refresh() {
        setIsLoading(true);
        try {
            const result = normalizeWorkspace(await DiscoverLocalEnvironment());
            setWorkspace(result);
            setMessage(result.warnings?.length ? result.warnings.join(' ') : undefined);
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

    function dropSkillByID(scopeID: string) {
        const scope = workspace?.scopes.find((item) => item.id === scopeID);
        if (scope) void dropSkill(scope);
    }

    function requestSkillDeletion(skill: domain.Skill) {
        setContextMenu(undefined);
        setPendingDeletion(skill);
    }

    function saveLaneOrder(group: LaneGroup, scopeIDs: string[]) {
        // Reorder locally first so the lane lands immediately, then persist.
        setWorkspace((current) => current && reorderScopes(current, scopeIDs));
        const sequence = ++laneSaveSequence.current;
        laneSaveQueue.current = laneSaveQueue.current.then(async () => {
            try {
                const result = group === 'agents' ? await SetAgentOrder(scopeIDs) : await ReorderProjects(scopeIDs.map((id) => id.replace(/^project:/, '')));
                if (sequence === laneSaveSequence.current) setWorkspace(normalizeWorkspace(result));
            } catch (error) {
                await refresh();
                setMessage(error instanceof Error ? error.message : String(error));
            }
        });
    }

    function moveLane(group: LaneGroup, scopeID: string, targetIndex: number) {
        const ids = (group === 'agents' ? agentLaneScopes : projectScopes).map((scope) => scope.id);
        const from = ids.indexOf(scopeID);
        if (from < 0 || targetIndex < 0 || targetIndex >= ids.length || targetIndex === from) return;
        ids.splice(from, 1);
        ids.splice(targetIndex, 0, scopeID);
        void saveLaneOrder(group, ids);
    }

    function laneReorder(group: LaneGroup, scope: domain.Scope, index: number): LaneReorder {
        return {
            group,
            draggingID: laneDrag?.group === group ? laneDrag.id : undefined,
            overID: laneDrag?.group === group ? laneDrag.overID : undefined,
            onStart: () => setLaneDrag({group, id: scope.id}),
            onOver: (overID) => setLaneDrag((current) => current && {...current, overID}),
            onDrop: (targetID) => {
                setLaneDrag(undefined);
                const lanes = group === 'agents' ? agentLaneScopes : projectScopes;
                const targetIndex = lanes.findIndex((lane) => lane.id === targetID);
                if (targetID && targetID !== scope.id && targetIndex >= 0) moveLane(group, scope.id, targetIndex);
            },
            onStep: (offset) => moveLane(group, scope.id, index + offset),
        };
    }

    async function setTrackedAgents(agentIDs: string[], successMessage: string) {
        try {
            setWorkspace(normalizeWorkspace(await SetTrackedAgents(agentIDs)));
            setMessage(successMessage);
            setIsAgentsDialogOpen(false);
        } catch (error) {
            setMessage(error instanceof Error ? error.message : String(error));
        }
    }

    function stopTrackingAgent(scope: domain.Scope) {
        const tracked = (workspace?.agents ?? []).filter((agent) => agent.tracked && agent.id !== scope.id).map((agent) => agent.id);
        void setTrackedAgents(tracked, `${scope.name} is no longer tracked. Use Agents to show it again.`);
    }

    async function syncGlobalSkills() {
        try {
            setWorkspace(normalizeWorkspace(await SyncGlobalSkills()));
            setMessage('Global skills synced to every installed agent.');
        } catch (error) {
            // Skills without a conflict were still synced, so show them before the error.
            await refresh();
            setMessage(error instanceof Error ? error.message : String(error));
        }
    }

    async function deleteSkill() {
        if (!pendingDeletion) return;
        setIsDeleting(true);
        try {
            setWorkspace(normalizeWorkspace(await DeleteSkill(pendingDeletion.path)));
            setMessage(`Deleted ${pendingDeletion.name}.`);
            setPendingDeletion(undefined);
        } catch (error) {
            setMessage(error instanceof Error ? error.message : 'Could not delete skill.');
        } finally {
            setIsDeleting(false);
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

    // Errors propagate to the Add skill dialog, which shows them next to the result.
    async function installRegistrySkill(skill: domain.RegistrySkill, target: domain.Scope) {
        const result = await InstallRegistrySkill(skill.source, skill.skillId, target.id);
        setWorkspace(normalizeWorkspace(result.workspace));
        setMessage(`${skill.name} from skills.sh was installed into ${target.name} via ${result.method}.`);
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

    async function changeSkillMode(skill: domain.Skill, mode: string) {
        try {
            setWorkspace(normalizeWorkspace(await SetSkillInvocationMode(skill.path, mode)));
            setMessage(`${skill.name} is now configured as ${mode}.`);
        } catch (error) {
            setMessage(error instanceof Error ? error.message : 'Could not configure skill usage.');
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

    // Agents the user stopped tracking are still reported (their skills count for
    // conflicts and shared-file warnings) but get no lane and are no destination.
    const scopes = (workspace?.scopes ?? []).filter((scope) => !scope.hidden);
    const visibleScopeIDs = new Set(scopes.map((scope) => scope.id));
    // The Global lane leads the Agents section: a skill dropped there reaches every installed agent.
    const globalScopes = scopes.filter((scope) => scope.kind === 'global');
    const agentLaneScopes = scopes.filter((scope) => scope.kind === 'agent');
    const projectScopes = scopes.filter((scope) => scope.kind === 'project');

    useEffect(() => {
        if (skillTargetID && scopes.some((scope) => scope.id === skillTargetID)) return;
        if (scopes.length) setSkillTargetID(scopes[0].id);
    }, [scopes, skillTargetID]);

    return (
        <main className="studio-shell" onClick={() => contextMenu && setContextMenu(undefined)}>
            <header className="studio-header">
                <div>
                    <h2>Agent Studio</h2>
                </div>
                <div className="header-actions"><button className="install-button" type="button" onClick={() => setIsInstallDialogOpen(true)}>Install skill</button><button className="refresh-button" type="button" onClick={() => setIsAgentsDialogOpen(true)} disabled={!workspace}>Agents</button><button className="refresh-button" type="button" onClick={() => void checkForUpdates()} disabled={isCheckingForUpdates}>{isCheckingForUpdates ? 'Checking...' : 'Check updates'}</button><button className="refresh-button" type="button" onClick={() => void refresh()} disabled={isLoading}>{isLoading ? 'Scanning...' : 'Refresh'}</button></div>
            </header>

            <section className="project-form">
                <div><label>Track a project</label></div>
                <button type="button" onClick={() => void selectProject()}>Select project folder</button>
                {selectedProjectPath ? <div className="selected-project"><span>Selected project</span><code title={selectedProjectPath}>{selectedProjectPath}</code></div> : null}
            </section>

            {message ? <p className="workspace-message">{message}</p> : null}

            <section className="workspace-section">
                <SectionHeading title="Agents"/>
                <div className="scope-grid">
                    {globalScopes.map((scope) => <ScopeLane key={scope.id} scope={scope} skills={skillsInScope(workspace, scope.id)} draggedSkill={draggedSkill} pointerDropScopeID={pointerDropScopeID} onDragStart={setDraggedSkill} onDrop={dropSkill} onPointerTarget={setPointerDropScopeID} onPointerDrop={dropSkillByID} onContextMenu={openContextMenu} onModeChange={changeSkillMode} onSyncGlobal={() => void syncGlobalSkills()} onAddSkill={() => { setAddSkillTarget(scope); setSkillSearch(''); }}/>)}
                    {agentLaneScopes.map((scope, index) => <ScopeLane key={scope.id} scope={scope} skills={skillsInScope(workspace, scope.id)} draggedSkill={draggedSkill} pointerDropScopeID={pointerDropScopeID} onDragStart={setDraggedSkill} onDrop={dropSkill} onPointerTarget={setPointerDropScopeID} onPointerDrop={dropSkillByID} onContextMenu={openContextMenu} onModeChange={changeSkillMode} onAddSkill={() => { setAddSkillTarget(scope); setSkillSearch(''); }} onStopTracking={() => stopTrackingAgent(scope)} reorder={laneReorder('agents', scope, index)}/>)}
                    {!agentLaneScopes.length && workspace ? <p className="empty-projects">No agents are tracked. Use <strong>Agents</strong> to choose which installed agents to show.</p> : null}
                </div>
            </section>

            <section className="workspace-section">
                <SectionHeading title="Selected projects"/>
                {projectScopes.length ? <div className="scope-grid project-grid">
                    {projectScopes.map((scope, index) => {
                        const projectID = scope.id.split(':')[1];
                        const project = workspace?.projects.find((item) => item.id === projectID);
                        return <ScopeLane key={scope.id} scope={scope} project={project} reorder={laneReorder('projects', scope, index)} skills={skillsInScope(workspace, scope.id)} draggedSkill={draggedSkill} pointerDropScopeID={pointerDropScopeID} onDragStart={setDraggedSkill} onDrop={dropSkill} onPointerTarget={setPointerDropScopeID} onPointerDrop={dropSkillByID} onContextMenu={openContextMenu} onModeChange={changeSkillMode} onRemoveProject={project ? () => void removeProject(project) : undefined} onAddSkill={() => { setAddSkillTarget(scope); setSkillSearch(''); }}/>;
                    })}
                </div> : <p className="empty-projects">Add a project path to create its `.agents/skills` destination.</p>}
            </section>

            {contextMenu ? <div className="context-menu" role="menu" style={{left: contextMenu.x, top: contextMenu.y}} onClick={(event) => event.stopPropagation()}>
                <button type="button" role="menuitem" onClick={() => requestSkillDeletion(contextMenu.skill)}>Delete skill</button>
                <strong>{contextMenu.skill.name}</strong>
            </div> : null}

            {addSkillTarget ? <AddSkillModal
                target={addSkillTarget}
                skills={(workspace?.skills ?? []).filter((skill) => visibleScopeIDs.has(skill.scopeId))}
                search={skillSearch}
                onSearch={setSkillSearch}
                onAdd={(skill) => void addSkillToProject(skill, addSkillTarget)}
                onInstallRegistry={(skill) => installRegistrySkill(skill, addSkillTarget)}
                onClose={() => setAddSkillTarget(undefined)}
            /> : null}
            {isAgentsDialogOpen && workspace ? <AgentsDialog agents={workspace.agents} onSave={(ids) => void setTrackedAgents(ids, 'Tracked agents updated.')} onClose={() => setIsAgentsDialogOpen(false)}/> : null}
            {isInstallDialogOpen ? <InstallSkillDialog url={skillURL} targetID={skillTargetID} scopes={scopes} isInstalling={isInstalling} onURLChange={setSkillURL} onTargetChange={setSkillTargetID} onSubmit={installFromURL} onClose={() => { if (!isInstalling) setIsInstallDialogOpen(false); }}/> : null}
            {availableUpdate ? <UpdateDialog update={availableUpdate} isUpdating={isUpdating} onInstall={() => void installUpdate()} onClose={() => { if (!isUpdating) setAvailableUpdate(undefined); }}/> : null}
            {pendingDeletion ? <DeleteSkillDialog skill={pendingDeletion} sharedWith={scopesSharingSkill(workspace, pendingDeletion)} isDeleting={isDeleting} onDelete={() => void deleteSkill()} onClose={() => { if (!isDeleting) setPendingDeletion(undefined); }}/> : null}
        </main>
    );
}

function ScopeLane({scope, project, skills, draggedSkill, pointerDropScopeID, onDragStart, onDrop, onPointerTarget, onPointerDrop, onContextMenu, onModeChange, onRemoveProject, onAddSkill, onSyncGlobal, onStopTracking, reorder}: {
    scope: domain.Scope;
    project?: domain.Project;
    skills: domain.Skill[];
    draggedSkill: domain.Skill | undefined;
    pointerDropScopeID: string | undefined;
    onDragStart: (skill: domain.Skill) => void;
    onDrop: (scope: domain.Scope) => void;
    onPointerTarget: (scopeID: string | undefined) => void;
    onPointerDrop: (scopeID: string) => void;
    onContextMenu: (skill: domain.Skill, x: number, y: number) => void;
    onModeChange: (skill: domain.Skill, mode: string) => void;
    onRemoveProject?: () => void;
    onAddSkill?: () => void;
    onSyncGlobal?: () => void;
    onStopTracking?: () => void;
    reorder?: LaneReorder;
}) {
    const [isDropTarget, setIsDropTarget] = useState(false);
    const [expandedSkills, setExpandedSkills] = useState<Set<string>>(new Set());
    const [projectMenuOpen, setProjectMenuOpen] = useState(false);
    const pointerDrag = useRef<{pointerID: number; skill: domain.Skill; startX: number; startY: number; active: boolean} | undefined>(undefined);
    const [pointerPreview, setPointerPreview] = useState<{name: string; x: number; y: number}>();
    const laneDrag = useRef<{pointerID: number; startX: number; startY: number; active: boolean} | undefined>(undefined);
    const gripRef = useRef<HTMLButtonElement>(null);
    const iconSource = scopeIcon(scope);

    function laneTarget(event: React.PointerEvent) {
        const lane = document.elementFromPoint(event.clientX, event.clientY)?.closest(`[data-reorder-group="${reorder?.group}"]`) as HTMLElement | null;
        return lane?.dataset.scopeId;
    }

    function startLaneDrag(event: React.PointerEvent<HTMLElement>) {
        if (!reorder || event.button !== 0) return;
        laneDrag.current = {pointerID: event.pointerId, startX: event.clientX, startY: event.clientY, active: false};
        event.currentTarget.setPointerCapture(event.pointerId);
    }

    function moveLaneDrag(event: React.PointerEvent<HTMLElement>) {
        const drag = laneDrag.current;
        if (!reorder || !drag || drag.pointerID !== event.pointerId) return;
        if (!drag.active && Math.hypot(event.clientX - drag.startX, event.clientY - drag.startY) < 4) return;
        if (!drag.active) {
            drag.active = true;
            reorder.onStart();
        }
        event.preventDefault();
        reorder.onOver(laneTarget(event));
    }

    function finishLaneDrag(event: React.PointerEvent<HTMLElement>) {
        const drag = laneDrag.current;
        if (!reorder || !drag || drag.pointerID !== event.pointerId) return;
        laneDrag.current = undefined;
        if (event.currentTarget.hasPointerCapture(event.pointerId)) event.currentTarget.releasePointerCapture(event.pointerId);
        if (drag.active) reorder.onDrop(laneTarget(event));
    }

    // Also bound to lostpointercapture: if the window loses focus mid-drag, the
    // lane must not stay stuck in its dragging state.
    function cancelLaneDrag(event: React.PointerEvent<HTMLElement>) {
        if (!reorder || !laneDrag.current || laneDrag.current.pointerID !== event.pointerId) return;
        laneDrag.current = undefined;
        reorder.onDrop(undefined);
    }

    function stepLane(event: React.KeyboardEvent<HTMLButtonElement>, offset: number) {
        if (!reorder) return;
        event.preventDefault();
        reorder.onStep(offset);
        // Moving the lane moves its DOM node; WebKit may drop focus, so restore it
        // and repeated arrow presses keep moving the same lane.
        requestAnimationFrame(() => gripRef.current?.focus());
    }

    function pointerTarget(event: React.PointerEvent) {
        return (document.elementFromPoint(event.clientX, event.clientY)?.closest('[data-scope-id]') as HTMLElement | null)?.dataset.scopeId;
    }

    function startPointerDrag(event: React.PointerEvent<HTMLElement>, skill: domain.Skill) {
        if (!usePointerDrag || event.button !== 0 || (event.target as HTMLElement).closest('button, select, input, textarea')) return;
        pointerDrag.current = {pointerID: event.pointerId, skill, startX: event.clientX, startY: event.clientY, active: false};
        event.currentTarget.setPointerCapture(event.pointerId);
    }

    function movePointerDrag(event: React.PointerEvent<HTMLElement>) {
        const drag = pointerDrag.current;
        if (!drag || drag.pointerID !== event.pointerId) return;
        if (!drag.active && Math.hypot(event.clientX - drag.startX, event.clientY - drag.startY) < 6) return;
        if (!drag.active) {
            drag.active = true;
            onDragStart(drag.skill);
        }
        event.preventDefault();
        setPointerPreview({name: drag.skill.name, x: event.clientX + 14, y: event.clientY + 14});
        onPointerTarget(pointerTarget(event));
    }

    function finishPointerDrag(event: React.PointerEvent<HTMLElement>) {
        const drag = pointerDrag.current;
        if (!drag || drag.pointerID !== event.pointerId) return;
        const targetID = drag.active ? pointerTarget(event) : undefined;
        pointerDrag.current = undefined;
        setPointerPreview(undefined);
        onPointerTarget(undefined);
        if (drag.active && targetID) onPointerDrop(targetID);
        if (event.currentTarget.hasPointerCapture(event.pointerId)) event.currentTarget.releasePointerCapture(event.pointerId);
    }

    function cancelPointerDrag(event: React.PointerEvent<HTMLElement>) {
        if (!pointerDrag.current || pointerDrag.current.pointerID !== event.pointerId) return;
        pointerDrag.current = undefined;
        setPointerPreview(undefined);
        onPointerTarget(undefined);
    }

    function toggleSkill(skillID: string) {
        setExpandedSkills((current) => {
            const next = new Set(current);
            if (next.has(skillID)) next.delete(skillID);
            else next.add(skillID);
            return next;
        });
    }

    const isReorderSource = reorder?.draggingID === scope.id;
    const isReorderTarget = Boolean(reorder?.draggingID) && reorder?.overID === scope.id && !isReorderSource;
    return <section
        data-scope-id={scope.id}
        data-reorder-group={reorder?.group}
        className={`scope-lane ${isDropTarget || pointerDropScopeID === scope.id ? 'is-drop-target' : ''} ${isReorderSource ? 'is-reorder-source' : ''} ${isReorderTarget ? 'is-reorder-target' : ''}`}
        onDragOver={(event) => { event.preventDefault(); setIsDropTarget(Boolean(draggedSkill && draggedSkill.scopeId !== scope.id)); }}
        onDragLeave={() => setIsDropTarget(false)}
        onDrop={(event) => { event.preventDefault(); setIsDropTarget(false); onDrop(scope); }}
    >
        <div className="scope-heading">
            <div>{reorder ? <button ref={gripRef} className="lane-grip" type="button" aria-label={`Move ${scope.name}`} title="Drag to reorder, or focus and use the arrow keys" onPointerDown={startLaneDrag} onPointerMove={moveLaneDrag} onPointerUp={finishLaneDrag} onPointerCancel={cancelLaneDrag} onLostPointerCapture={cancelLaneDrag} onKeyDown={(event) => {
                if (event.key === 'ArrowLeft' || event.key === 'ArrowUp') stepLane(event, -1);
                if (event.key === 'ArrowRight' || event.key === 'ArrowDown') stepLane(event, 1);
            }}>⠿</button> : null}<span className={`scope-icon ${scope.kind}`}>{iconSource ? <img src={iconSource} alt=""/> : scope.kind === 'global' ? 'G' : 'P'}</span><strong>{scope.name}</strong></div>
            <div className="scope-actions">
                <span>{skills.length}</span>
                {onAddSkill || onRemoveProject || onStopTracking ? <div className="project-menu-wrap">
                    <button className="project-menu-button" type="button" aria-label={`Menu for ${scope.name}`} aria-expanded={projectMenuOpen} onClick={(event) => { event.stopPropagation(); setProjectMenuOpen((open) => !open); }}>⋮</button>
                    {projectMenuOpen ? <div className="project-menu" role="menu" onClick={(event) => event.stopPropagation()}>{onAddSkill ? <button type="button" role="menuitem" onClick={() => { setProjectMenuOpen(false); onAddSkill(); }}>Add skill</button> : null}{scope.kind === 'project' && onRemoveProject ? <button type="button" role="menuitem" onClick={() => { setProjectMenuOpen(false); onRemoveProject(); }}>Stop tracking</button> : null}{onStopTracking ? <button type="button" role="menuitem" onClick={() => { setProjectMenuOpen(false); onStopTracking(); }}>Stop tracking</button> : null}</div> : null}
                </div> : null}
            </div>
        </div>
        <p className="scope-kind">{scopeKindLabel(scope)}</p>
        {project ? <code className="project-path" title={project.path}>{project.path}</code> : null}
        {onSyncGlobal ? <button className="sync-global-button" type="button" title="Copy Global skills to installed agents that cannot read ~/.agents/skills" onClick={onSyncGlobal}>Sync to agents</button> : null}
        {scope.kind === 'project' && onRemoveProject ? <button className="stop-tracking-button" type="button" onClick={onRemoveProject}>Stop tracking</button> : null}
        <div className="skill-stack">
            {skills.map((skill) => {
                const isExpanded = expandedSkills.has(skill.id);
                return <article className={`skill-card ${isExpanded ? 'is-expanded' : ''}`} key={skill.id} draggable={!usePointerDrag} onDragStart={(event) => { event.dataTransfer.setData('text/plain', skill.path); event.dataTransfer.effectAllowed = 'copy'; onDragStart(skill); }} onDragEnd={() => setIsDropTarget(false)} onPointerDown={(event) => startPointerDrag(event, skill)} onPointerMove={movePointerDrag} onPointerUp={finishPointerDrag} onPointerCancel={cancelPointerDrag} onContextMenu={(event) => { event.preventDefault(); onContextMenu(skill, event.clientX, event.clientY); }}>
                    <button className="skill-card-toggle" type="button" aria-expanded={isExpanded} onClick={() => toggleSkill(skill.id)}>
                        <strong>{skill.name}</strong>
                        <span className="skill-card-action"><span className="skill-state-count">{skill.states?.length ?? 0}</span><span className="expand-icon" aria-hidden="true">{isExpanded ? '−' : '+'}</span></span>
                    </button>
                    {isExpanded ? <div className="skill-card-details">
                        <p>{skill.description}</p>
                        <small>{skill.path}</small>
                        <div className="skill-states">{(skill.states ?? []).map((state) => <span className={state} key={state}>{state}</span>)}</div>
                        <label className="skill-mode-label">Usage<select value={skill.invocationMode || 'automatic'} onChange={(event) => void onModeChange(skill, event.target.value)}><option value="automatic">Automatic</option><option value="always">Always active</option><option value="explicit">Explicit</option><option value="disabled">Disabled</option></select></label>
                    </div> : null}
                    <span className="drag-handle" aria-label="Drag to copy">::</span>
                </article>;
            })}
            {!skills.length ? <p className="lane-placeholder">Drop a skill here to copy it.</p> : null}
        </div>
        {pointerPreview ? <div className="pointer-drag-preview" style={{left: pointerPreview.x, top: pointerPreview.y}}>{pointerPreview.name}</div> : null}
    </section>;
}

function scopeKindLabel(scope: domain.Scope) {
    if (scope.kind === 'global') return 'Available to every installed agent';
    if (scope.kind === 'project') return 'Project skills';
    return 'Agent skills';
}

function scopeIcon(scope: domain.Scope) {
    if (scope.kind !== 'agent') return undefined;
    if (scope.provider === 'claude') return claudeIcon;
    if (scope.provider === 'codex') return codexIcon;
    if (scope.provider === 'opencode') return openCodeIcon;
    return undefined;
}

function SectionHeading({title}: {title: string}) {
    return <div className="section-heading"><h2>{title}</h2></div>;
}

const REGISTRY_MINIMUM_QUERY = 2;
const REGISTRY_SEARCH_DELAY_MS = 350;

function AddSkillModal({target, skills, search, onSearch, onAdd, onInstallRegistry, onClose}: {
    target: domain.Scope;
    skills: domain.Skill[];
    search: string;
    onSearch: (value: string) => void;
    onAdd: (skill: domain.Skill) => void;
    onInstallRegistry: (skill: domain.RegistrySkill) => Promise<void>;
    onClose: () => void;
}) {
    const normalizedSearch = search.trim().toLowerCase();
    const targetNames = new Set(skills.filter((skill) => skill.scopeId === target.id).map((skill) => skill.name.toLowerCase()));
    const availableSkills = skills.filter((skill) => !normalizedSearch || `${skill.name} ${skill.description}`.toLowerCase().includes(normalizedSearch));
    const searchesRegistry = normalizedSearch.length >= REGISTRY_MINIMUM_QUERY;
    const [registryResults, setRegistryResults] = useState<domain.RegistrySkill[]>([]);
    const [registryStatus, setRegistryStatus] = useState<'idle' | 'loading' | 'done' | 'error'>('idle');
    const [registryError, setRegistryError] = useState('');
    const [installingID, setInstallingID] = useState<string>();
    const [installError, setInstallError] = useState<{id: string; message: string}>();

    // Local skills filter as you type; skills.sh is queried once typing pauses, and
    // a response for an older query is ignored.
    useEffect(() => {
        if (!searchesRegistry) {
            setRegistryResults([]);
            setRegistryStatus('idle');
            return;
        }
        let isCurrent = true;
        setRegistryStatus('loading');
        const timer = window.setTimeout(() => {
            SearchRegistrySkills(search.trim())
                .then((results) => {
                    if (!isCurrent) return;
                    setRegistryResults(results ?? []);
                    setRegistryStatus('done');
                })
                .catch((error) => {
                    if (!isCurrent) return;
                    setRegistryError(error instanceof Error ? error.message : String(error));
                    setRegistryStatus('error');
                });
        }, REGISTRY_SEARCH_DELAY_MS);
        return () => {
            isCurrent = false;
            window.clearTimeout(timer);
        };
    }, [search, searchesRegistry]);

    async function install(skill: domain.RegistrySkill) {
        setInstallingID(skill.id);
        setInstallError(undefined);
        try {
            await onInstallRegistry(skill);
        } catch (error) {
            setInstallError({id: skill.id, message: error instanceof Error ? error.message : String(error)});
        } finally {
            setInstallingID(undefined);
        }
    }

    return <div className="modal-backdrop" role="presentation" onClick={() => { if (!installingID) onClose(); }}>
        <section className="skill-modal" role="dialog" aria-modal="true" aria-labelledby="add-skill-title" onClick={(event) => event.stopPropagation()}>
            <div className="modal-heading"><h2 id="add-skill-title">Add skill to {target.name}</h2><button className="modal-close" type="button" aria-label="Close" onClick={onClose} disabled={Boolean(installingID)}>×</button></div>
            <input className="skill-search" value={search} onChange={(event) => onSearch(event.target.value)} placeholder="Search your skills, or type a name to search skills.sh" autoFocus/>
            <div className="modal-skill-list">
                {searchesRegistry ? <h3 className="modal-skill-section">On this computer</h3> : null}
                {availableSkills.map((skill) => {
                    const alreadyAdded = targetNames.has(skill.name.toLowerCase());
                    return <article className="modal-skill-row" key={`${skill.id}-${skill.scopeId}`}><div><strong>{skill.name}</strong><p className="modal-skill-description" title={skill.description}>{skill.description}</p><small>{skill.scopeId}</small></div><button type="button" disabled={alreadyAdded} onClick={() => onAdd(skill)}>{alreadyAdded ? 'Added' : 'Add'}</button></article>;
                })}
                {!availableSkills.length ? <p className="empty-state">No skills match your search.</p> : null}
                {searchesRegistry ? <>
                    <h3 className="modal-skill-section">From skills.sh</h3>
                    {registryStatus === 'loading' ? <p className="empty-state">Searching skills.sh...</p> : null}
                    {registryStatus === 'error' ? <p className="empty-state registry-error">{registryError}</p> : null}
                    {registryStatus === 'done' && !registryResults.length ? <p className="empty-state">No skills.sh results for “{search.trim()}”.</p> : null}
                    {registryStatus === 'done' ? registryResults.map((skill) => {
                        // A same-named folder already exists in the target, so installing would
                        // be refused; it may be a different skill, hence not "Installed".
                        const nameInUse = targetNames.has(skill.name.toLowerCase());
                        const isInstalling = installingID === skill.id;
                        return <article className="modal-skill-row" key={skill.id}>
                            <div>
                                <strong>{skill.name}</strong>
                                <p><a href={skill.url} onClick={(event) => { event.preventDefault(); BrowserOpenURL(skill.url); }}>{skill.source}</a> · {formatInstalls(skill.installs)} installs</p>
                                {installError?.id === skill.id ? <small className="registry-error">{installError.message}</small> : null}
                            </div>
                            <button type="button" disabled={nameInUse || Boolean(installingID)} title={nameInUse ? `${target.name} already has a skill named ${skill.name}` : undefined} onClick={() => void install(skill)}>{nameInUse ? 'Name in use' : isInstalling ? 'Installing...' : 'Install'}</button>
                        </article>;
                    }) : null}
                </> : null}
            </div>
        </section>
    </div>;
}

function formatInstalls(installs: number) {
    return new Intl.NumberFormat('en', {notation: 'compact', maximumFractionDigits: 1}).format(installs);
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
            <div className="modal-heading"><h2 id="install-skill-title">Install skill</h2><button className="modal-close" type="button" aria-label="Close" onClick={onClose} disabled={isInstalling}>×</button></div>
            <p className="install-help">Paste a public GitHub, GitLab, or Bitbucket repository URL, or an <code>npx skills add</code> command. Agent Studio will use Git when available and ZIP otherwise.</p>
            <form className="install-dialog-form" onSubmit={onSubmit}>
                <label htmlFor="skill-url">Repository URL or npx command</label>
                <input id="skill-url" value={url} onChange={(event) => onURLChange(event.target.value)} placeholder="https://github.com/owner/repository/tree/main/skills/my-skill" autoFocus required/>
                <small className="install-hint">Also accepts <code>npx skills add https://github.com/owner/repository --skill my-skill</code></small>
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
            <div className="modal-heading"><h2 id="update-title">Update to {update.latestVersion}</h2><button className="modal-close" type="button" aria-label="Close" onClick={onClose} disabled={isUpdating}>×</button></div>
            <p className="install-help">You are using {update.currentVersion}. The installer will be downloaded, verified, and installed automatically.</p>
            {update.releaseNotes ? <pre className="update-notes">{update.releaseNotes}</pre> : null}
            <div className="install-dialog-actions"><button type="button" className="modal-secondary" onClick={onClose} disabled={isUpdating}>Later</button><button type="button" className="install-submit" onClick={onInstall} disabled={isUpdating}>{isUpdating ? 'Installing...' : 'Download and install'}</button></div>
        </section>
    </div>;
}

function DeleteSkillDialog({skill, sharedWith, isDeleting, onDelete, onClose}: {
    skill: domain.Skill;
    sharedWith: domain.Scope[];
    isDeleting: boolean;
    onDelete: () => void;
    onClose: () => void;
}) {
    return <div className="modal-backdrop" role="presentation" onClick={onClose}>
        <section className="delete-modal" role="dialog" aria-modal="true" aria-labelledby="delete-skill-title" onClick={(event) => event.stopPropagation()}>
            <div className="modal-heading"><h2 id="delete-skill-title">Delete {skill.name}?</h2><button className="modal-close" type="button" aria-label="Close" onClick={onClose} disabled={isDeleting}>×</button></div>
            {skill.scopeId === 'global' ? <p>This is a Global skill, so it is removed from every agent, including the copy Agent Studio made for Claude Code. Skills an agent has on its own are not affected.</p> : sharedWith.length ? <p>This file is shared by {sharedWith.map((scope) => scope.hidden ? `${scope.name} (hidden)` : scope.name).join(' and ')}, so it is removed from all of them. Copies elsewhere are not affected.</p> : <p>This permanently removes only this copy. Copies in other agents or projects are not affected.</p>}
            <code title={skill.path}>{skill.path}</code>
            <div className="install-dialog-actions"><button type="button" className="modal-secondary" onClick={onClose} disabled={isDeleting}>Cancel</button><button type="button" className="delete-confirm" onClick={onDelete} disabled={isDeleting}>{isDeleting ? 'Deleting...' : 'Delete skill'}</button></div>
        </section>
    </div>;
}

// Agents that read the same skill directory (OpenCode v1 and v2) list the same file.
function scopesSharingSkill(workspace: domain.DiscoveryResult | undefined, skill: domain.Skill) {
    const scopeIDs = new Set(workspace?.skills.filter((item) => item.path === skill.path).map((item) => item.scopeId));
    if (scopeIDs.size < 2) return [];
    return workspace?.scopes.filter((scope) => scopeIDs.has(scope.id)) ?? [];
}

// reorderScopes applies a new order to one group of lanes, leaving the others in place.
function reorderScopes(workspace: domain.DiscoveryResult, orderedIDs: string[]): domain.DiscoveryResult {
    const moved = new Set(orderedIDs);
    const ordered = orderedIDs.map((id) => workspace.scopes.find((scope) => scope.id === id)).filter((scope): scope is domain.Scope => Boolean(scope));
    let next = 0;
    const scopes = workspace.scopes.map((scope) => moved.has(scope.id) ? ordered[next++] : scope);
    return {...workspace, scopes} as domain.DiscoveryResult;
}

function AgentsDialog({agents, onSave, onClose}: {
    agents: domain.Agent[];
    onSave: (agentIDs: string[]) => void;
    onClose: () => void;
}) {
    const [selected, setSelected] = useState(() => new Set(agents.filter((agent) => agent.tracked).map((agent) => agent.id)));

    function toggle(agentID: string) {
        setSelected((current) => {
            const next = new Set(current);
            if (next.has(agentID)) next.delete(agentID);
            else next.add(agentID);
            return next;
        });
    }

    return <div className="modal-backdrop" role="presentation" onClick={onClose}>
        <section className="install-modal" role="dialog" aria-modal="true" aria-labelledby="agents-title" onClick={(event) => event.stopPropagation()}>
            <div className="modal-heading"><h2 id="agents-title">Agents</h2><button className="modal-close" type="button" aria-label="Close" onClick={onClose}>×</button></div>
            <p className="install-help">Choose which installed agents Agent Studio shows. Hiding an agent never changes its files or configuration, and Global skills still reach it.</p>
            <div className="agent-choice-list">
                {agents.map((agent) => <label className="agent-choice" key={agent.id}>
                    <input type="checkbox" checked={selected.has(agent.id)} onChange={() => toggle(agent.id)}/>
                    <span><strong>{agent.name}{agent.version ? ` · ${agent.version}` : ''}</strong><small title={agent.commandPath}>{agent.commandPath}</small></span>
                </label>)}
                {!agents.length ? <p className="empty-state">No supported agent is installed.</p> : null}
            </div>
            <div className="install-dialog-actions"><button type="button" className="modal-secondary" onClick={onClose}>Cancel</button><button type="button" className="install-submit" onClick={() => onSave(agents.filter((agent) => selected.has(agent.id)).map((agent) => agent.id))}>Save</button></div>
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
        return <main className="fatal-error"><h1>Could not render the workspace</h1><p>{this.state.error.message}</p><button onClick={() => window.location.reload()}>Reload</button></main>;
    }
}

export default function RootApp() {
    return <ErrorBoundary><App /></ErrorBoundary>;
}
