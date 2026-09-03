import {useEffect, useState} from 'react';
import {DiscoverLocalEnvironment} from '../wailsjs/go/main/App';
import {domain} from '../wailsjs/go/models';
import './App.css';

function App() {
    const [inventory, setInventory] = useState<domain.DiscoveryResult>();
    const [error, setError] = useState<string>();
    const [isLoading, setIsLoading] = useState(true);

    async function refreshInventory() {
        setIsLoading(true);
        setError(undefined);
        try {
            setInventory(await DiscoverLocalEnvironment());
        } catch (reason) {
            setError(reason instanceof Error ? reason.message : 'Could not inspect the local environment.');
        } finally {
            setIsLoading(false);
        }
    }

    useEffect(() => {
        void refreshInventory();
    }, []);

    return (
        <main className="app-shell">
            <header className="topbar">
                <div>
                    <p className="eyebrow">LOCAL-FIRST AGENT CONFIGURATION</p>
                    <h1>Agent Studio</h1>
                </div>
                <button className="refresh-button" onClick={() => void refreshInventory()} disabled={isLoading}>
                    {isLoading ? 'Scanning...' : 'Refresh inventory'}
                </button>
            </header>

            {error ? <p className="error-message">{error}</p> : null}

            <section className="summary-grid" aria-label="Environment summary">
                <Summary label="Agents found" value={inventory?.agents.filter((agent) => agent.status !== 'not found').length ?? 0}/>
                <Summary label="Skills found" value={inventory?.skills.length ?? 0}/>
                <Summary label="Config files" value={inventory?.configFiles.length ?? 0}/>
            </section>

            <section className="content-grid">
                <section className="panel">
                    <div className="panel-heading">
                        <div>
                            <p className="section-kicker">TERMINAL AGENTS</p>
                            <h2>Detected agents</h2>
                        </div>
                        <span className="read-only">Read only</span>
                    </div>
                    <div className="agent-list">
                        {inventory?.agents.map((agent) => (
                            <article className="agent-row" key={agent.id}>
                                <span className={`agent-mark ${agent.provider}`}>{agent.name.slice(0, 1)}</span>
                                <div className="agent-details">
                                    <strong>{agent.name}</strong>
                                    <span>{agent.status === 'configured' ? 'Configuration found' : agent.status === 'installed' ? 'Command found' : 'Not found'}</span>
                                </div>
                                <span className={`status ${agent.status.replace(' ', '-')}`}>{agent.status}</span>
                            </article>
                        ))}
                    </div>
                </section>

                <section className="panel skills-panel">
                    <div className="panel-heading">
                        <div>
                            <p className="section-kicker">SKILL CATALOG</p>
                            <h2>Local skills</h2>
                        </div>
                    </div>
                    {inventory?.skills.length ? (
                        <div className="skill-list">
                            {inventory.skills.map((skill) => (
                                <article className="skill-row" key={skill.id}>
                                    <div>
                                        <strong>{skill.name}</strong>
                                        <p>{skill.description}</p>
                                    </div>
                                    <div className="source-list" aria-label={`Available for ${skill.sources.join(', ')}`}>
                                        {skill.sources.map((source) => <span key={source}>{source}</span>)}
                                    </div>
                                </article>
                            ))}
                        </div>
                    ) : <p className="empty-state">No `SKILL.md` files were found in the known local directories.</p>}
                </section>
            </section>
        </main>
    );
}

function Summary({label, value}: {label: string; value: number}) {
    return <div className="summary-card"><strong>{value}</strong><span>{label}</span></div>;
}

export default App;
