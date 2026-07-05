import React, { useState, useEffect, useMemo } from 'react';
import {
  Shield,
  Plus,
  Trash2,
  Eye,
  EyeOff,
  Search,
  Lock,
  X,
  FolderPlus,
  Copy,
  Key,
  Activity,
  Check,
  SearchCode,
  Sparkles
} from 'lucide-react';
import './App.css';
import { BootstrapModal } from './BootstrapModal';

interface Environment {
  name: string;
  has_secrets: boolean;
}

interface Project {
  name: string;
  environments: Environment[];
}

const BACKEND_URL = import.meta.env.DEV ? 'http://localhost:4554' : '';

function App() {
  const [token, setToken] = useState<string>('');
  const [projects, setProjects] = useState<Project[]>([]);
  const [statusInfo, setStatusInfo] = useState<{ db_path: string; projects_count: number } | null>(null);
  const [showDbPath, setShowDbPath] = useState<boolean>(false);

  // Selected state
  const [selectedProject, setSelectedProject] = useState<string>('');
  const [selectedEnv, setSelectedEnv] = useState<string>('');
  const [expandedProjects, setExpandedProjects] = useState<Record<string, boolean>>({});

  // Secrets state
  const [secrets, setSecrets] = useState<Record<string, string>>({});
  const [visibleSecrets, setVisibleSecrets] = useState<Record<string, boolean>>({});
  const [showAllSecrets, setShowAllSecrets] = useState<boolean>(false);
  const [searchQuery, setSearchQuery] = useState<string>('');
  const [copiedKey, setCopiedKey] = useState<string | null>(null);

  // Modal states
  const [showProjModal, setShowProjModal] = useState<boolean>(false);
  const [newProjName, setNewProjName] = useState<string>('');

  const [showEnvModal, setShowEnvModal] = useState<boolean>(false);
  const [newEnvName, setNewEnvName] = useState<string>('');

  const [showSecretModal, setShowSecretModal] = useState<boolean>(false);
  const [secretKey, setSecretKey] = useState<string>('');
  const [secretValue, setSecretValue] = useState<string>('');
  const [isEditingSecret, setIsEditingSecret] = useState<boolean>(false);

  // .env Import States
  const [showImportModal, setShowImportModal] = useState<boolean>(false);
  const [showBootstrapModal, setShowBootstrapModal] = useState<boolean>(false);
  const [importTab, setImportTab] = useState<'upload' | 'paste'>('upload');
  const [importText, setImportText] = useState<string>('');
  const [dragActive, setDragActive] = useState<boolean>(false);
  const [importStrategy, setImportStrategy] = useState<'merge' | 'overwrite'>('merge');

  const [errorMsg, setErrorMsg] = useState<string>('');
  const [successMsg, setSuccessMsg] = useState<string>('');

  // .env parsing utility
  const parseEnv = (text: string): Record<string, string> => {
    const result: Record<string, string> = {};
    const lines = text.split(/\r?\n/);

    let currentKey: string | null = null;
    let currentValue: string[] = [];
    let quoteChar: string | null = null;

    // Helper to strip trailing comments from a value
    const stripComments = (val: string): string => {
      if (val.startsWith('"')) {
        const closingIdx = val.indexOf('"', 1);
        if (closingIdx !== -1) {
          return val.substring(0, closingIdx + 1);
        }
      } else if (val.startsWith("'")) {
        const closingIdx = val.indexOf("'", 1);
        if (closingIdx !== -1) {
          return val.substring(0, closingIdx + 1);
        }
      }

      const commentIdx = val.indexOf('#');
      if (commentIdx !== -1) {
        return val.substring(0, commentIdx);
      }
      return val;
    };

    // Helper to parse/clean a single-line or joined multiline value
    const parseValue = (val: string, qChar: string | null): string => {
      let content = stripComments(val).trim();
      if (!qChar) return content;

      if (content.startsWith(qChar) && content.endsWith(qChar)) {
        content = content.substring(1, content.length - 1);
      } else {
        if (content.startsWith(qChar)) content = content.substring(1);
        if (content.endsWith(qChar)) content = content.substring(0, content.length - 1);
      }

      if (qChar === '"') {
        content = content
          .replace(/\\n/g, '\n')
          .replace(/\\r/g, '\r')
          .replace(/\\t/g, '\t')
          .replace(/\\"/g, '"')
          .replace(/\\\\/g, '\\');
      } else if (qChar === "'") {
        content = content
          .replace(/\\'/g, "'")
          .replace(/\\\\/g, '\\');
      }
      return content;
    };

    for (let line of lines) {
      if (currentKey !== null) {
        currentValue.push(line);

        let isEnd = false;
        let idx = -1;
        while (true) {
          idx = line.indexOf(quoteChar!, idx + 1);
          if (idx === -1) break;
          let backslashes = 0;
          for (let i = idx - 1; i >= 0; i--) {
            if (line[i] === '\\') backslashes++;
            else break;
          }
          if (backslashes % 2 === 0) {
            const trailing = line.substring(idx + 1).trim();
            if (trailing === '' || trailing.startsWith('#')) {
              isEnd = true;
              break;
            }
          }
        }

        if (isEnd) {
          const fullVal = currentValue.join('\n');
          result[currentKey] = parseValue(fullVal, quoteChar);
          currentKey = null;
          currentValue = [];
          quoteChar = null;
        }
        continue;
      }

      const trimmed = line.trim();
      if (!trimmed || trimmed.startsWith('#')) continue;

      const eqIdx = line.indexOf('=');
      if (eqIdx === -1) continue;

      const key = line.substring(0, eqIdx).trim();
      let val = line.substring(eqIdx + 1).trim();

      if (!key || key.startsWith('#')) continue;

      if (val.startsWith('"') || val.startsWith("'")) {
        const q = val[0];
        let closedIdx = -1;
        let idx = 0;
        while (true) {
          idx = val.indexOf(q, idx + 1);
          if (idx === -1) break;
          let backslashes = 0;
          for (let i = idx - 1; i >= 0; i--) {
            if (val[i] === '\\') backslashes++;
            else break;
          }
          if (backslashes % 2 === 0) {
            const trailing = val.substring(idx + 1).trim();
            if (trailing === '' || trailing.startsWith('#')) {
              closedIdx = idx;
              break;
            }
          }
        }

        if (closedIdx !== -1) {
          result[key] = parseValue(val.substring(0, closedIdx + 1), q);
        } else {
          currentKey = key;
          currentValue = [val];
          quoteChar = q;
        }
      } else {
        result[key] = parseValue(val, null);
      }
    }

    if (currentKey !== null) {
      result[currentKey] = parseValue(currentValue.join('\n'), quoteChar);
    }

    return result;
  };

  const parsedImportSecrets = useMemo(() => {
    return parseEnv(importText);
  }, [importText]);

  const handleFileChange = (e: React.ChangeEvent<HTMLInputElement>) => {
    const file = e.target.files?.[0];
    if (!file) return;
    readFileContent(file);
  };

  const readFileContent = (file: File) => {
    const reader = new FileReader();
    reader.onload = (event) => {
      const text = event.target?.result as string;
      setImportText(text);
      setImportTab('paste'); // Automatically switch to view/edit tab to preview
    };
    reader.readAsText(file);
  };

  const handleDrag = (e: React.DragEvent) => {
    e.preventDefault();
    e.stopPropagation();
    if (e.type === "dragenter" || e.type === "dragover") {
      setDragActive(true);
    } else if (e.type === "dragleave") {
      setDragActive(false);
    }
  };

  const handleDrop = (e: React.DragEvent) => {
    e.preventDefault();
    e.stopPropagation();
    setDragActive(false);

    const file = e.dataTransfer.files?.[0];
    if (file) {
      readFileContent(file);
    }
  };


  // 1. Token Initialization
  useEffect(() => {
    const params = new URLSearchParams(window.location.search);
    const urlToken = params.get('token');
    const savedToken = sessionStorage.getItem('shroudenv_token');

    const activeToken = urlToken || savedToken || '';
    if (activeToken) {
      setToken(activeToken);
      sessionStorage.setItem('shroudenv_token', activeToken);
    }
  }, []);

  // 2. Fetch projects and status
  const fetchStatusAndProjects = async (activeToken = token) => {
    if (!activeToken) return;

    const headers = { 'Authorization': `Bearer ${activeToken}` };

    try {
      // Status
      const statusRes = await fetch(`${BACKEND_URL}/api/status`, { headers });
      if (statusRes.ok) {
        const statusData = await statusRes.json();
        setStatusInfo(statusData);
      }

      // Projects
      const projectsRes = await fetch(`${BACKEND_URL}/api/projects`, { headers });
      if (projectsRes.ok) {
        const projectsData = await projectsRes.json();
        setProjects(projectsData);

        // Auto-select first project & env if none selected
        if (projectsData.length > 0) {
          const firstProj = projectsData[0];
          setExpandedProjects(prev => ({ ...prev, [firstProj.name]: true }));

          if (!selectedProject) {
            setSelectedProject(firstProj.name);
            if (firstProj.environments.length > 0) {
              setSelectedEnv(firstProj.environments[0].name);
            }
          }
        }
      } else if (projectsRes.status === 401) {
        setErrorMsg('Unauthorized: Invalid API token.');
      }
    } catch (err) {
      setErrorMsg('Failed to connect to the backend server.');
    }
  };

  useEffect(() => {
    if (token) {
      fetchStatusAndProjects(token);
    }
  }, [token]);

  // 3. Fetch secrets when selected project or environment changes
  const fetchSecrets = async () => {
    if (!token || !selectedProject || !selectedEnv) {
      setSecrets({});
      return;
    }

    const headers = { 'Authorization': `Bearer ${token}` };
    try {
      const url = `${BACKEND_URL}/api/projects/${encodeURIComponent(selectedProject)}/envs/${encodeURIComponent(selectedEnv)}/secrets`;
      const res = await fetch(url, { headers });
      if (res.ok) {
        const data = await res.json();
        setSecrets(data);
      } else {
        const text = await res.text();
        setErrorMsg(`Failed to load secrets: ${text}`);
      }
    } catch (err) {
      setErrorMsg('Network error while loading secrets.');
    }
  };

  useEffect(() => {
    fetchSecrets();
  }, [selectedProject, selectedEnv, token]);

  // Toast auto-clear
  useEffect(() => {
    if (errorMsg) {
      const timer = setTimeout(() => setErrorMsg(''), 5000);
      return () => clearTimeout(timer);
    }
  }, [errorMsg]);

  useEffect(() => {
    if (successMsg) {
      const timer = setTimeout(() => setSuccessMsg(''), 3000);
      return () => clearTimeout(timer);
    }
  }, [successMsg]);

  // 4. Create Project
  const handleCreateProject = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!newProjName.trim()) return;

    try {
      const res = await fetch(`${BACKEND_URL}/api/projects`, {
        method: 'POST',
        headers: {
          'Content-Type': 'application/json',
          'Authorization': `Bearer ${token}`
        },
        body: JSON.stringify({ name: newProjName.trim() })
      });

      if (res.ok) {
        setSuccessMsg(`Project "${newProjName}" created.`);
        setNewProjName('');
        setShowProjModal(false);
        fetchStatusAndProjects();
      } else {
        const text = await res.text();
        setErrorMsg(text || 'Failed to create project.');
      }
    } catch (err) {
      setErrorMsg('Connection error.');
    }
  };

  // 5. Create Environment
  const handleCreateEnv = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!newEnvName.trim() || !selectedProject) return;

    try {
      const url = `${BACKEND_URL}/api/projects/${encodeURIComponent(selectedProject)}/envs`;
      const res = await fetch(url, {
        method: 'POST',
        headers: {
          'Content-Type': 'application/json',
          'Authorization': `Bearer ${token}`
        },
        body: JSON.stringify({ name: newEnvName.trim() })
      });

      if (res.ok) {
        setSuccessMsg(`Environment "${newEnvName}" created inside "${selectedProject}".`);
        setNewEnvName('');
        setShowEnvModal(false);
        fetchStatusAndProjects();
        setSelectedEnv(newEnvName.trim());
      } else {
        const text = await res.text();
        setErrorMsg(text || 'Failed to create environment.');
      }
    } catch (err) {
      setErrorMsg('Connection error.');
    }
  };

  // .env Confirm Import
  const handleConfirmImport = async (e: React.FormEvent) => {
    e.preventDefault();
    if (Object.keys(parsedImportSecrets).length === 0) return;

    let finalSecrets = {};
    if (importStrategy === 'merge') {
      finalSecrets = { ...secrets, ...parsedImportSecrets };
    } else {
      finalSecrets = { ...parsedImportSecrets };
    }

    try {
      const url = `${BACKEND_URL}/api/projects/${encodeURIComponent(selectedProject)}/envs/${encodeURIComponent(selectedEnv)}/secrets`;
      const res = await fetch(url, {
        method: 'POST',
        headers: {
          'Content-Type': 'application/json',
          'Authorization': `Bearer ${token}`
        },
        body: JSON.stringify({ secrets: finalSecrets })
      });

      if (res.ok) {
        setSuccessMsg(`.env file imported successfully (${Object.keys(parsedImportSecrets).length} secrets).`);
        setShowImportModal(false);
        setImportText('');
        fetchSecrets();
        fetchStatusAndProjects();
      } else {
        const text = await res.text();
        setErrorMsg(text || 'Failed to import secrets.');
      }
    } catch (err) {
      setErrorMsg('Connection error during import.');
    }
  };


  // 6. Set (Create/Update) Secret
  const handleSetSecret = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!secretKey.trim() || !selectedProject || !selectedEnv) return;

    const updatedSecrets = { ...secrets, [secretKey.trim()]: secretValue };

    try {
      const url = `${BACKEND_URL}/api/projects/${encodeURIComponent(selectedProject)}/envs/${encodeURIComponent(selectedEnv)}/secrets`;
      const res = await fetch(url, {
        method: 'POST',
        headers: {
          'Content-Type': 'application/json',
          'Authorization': `Bearer ${token}`
        },
        body: JSON.stringify({ secrets: updatedSecrets })
      });

      if (res.ok) {
        setSuccessMsg(isEditingSecret ? 'Secret updated.' : 'Secret added.');
        setSecretKey('');
        setSecretValue('');
        setShowSecretModal(false);
        fetchSecrets();
        fetchStatusAndProjects(); // Update HasSecrets indicators
      } else {
        const text = await res.text();
        setErrorMsg(text || 'Failed to save secret.');
      }
    } catch (err) {
      setErrorMsg('Connection error.');
    }
  };

  // 7. Delete Secret
  const handleDeleteSecret = async (keyToDelete: string) => {
    if (!window.confirm(`Are you sure you want to delete secret "${keyToDelete}"?`)) return;

    const updatedSecrets = { ...secrets };
    delete updatedSecrets[keyToDelete];

    try {
      const url = `${BACKEND_URL}/api/projects/${encodeURIComponent(selectedProject)}/envs/${encodeURIComponent(selectedEnv)}/secrets`;
      const res = await fetch(url, {
        method: 'POST',
        headers: {
          'Content-Type': 'application/json',
          'Authorization': `Bearer ${token}`
        },
        body: JSON.stringify({ secrets: updatedSecrets })
      });

      if (res.ok) {
        setSuccessMsg('Secret deleted.');
        fetchSecrets();
        fetchStatusAndProjects(); // Update HasSecrets indicators
      } else {
        const text = await res.text();
        setErrorMsg(text || 'Failed to delete secret.');
      }
    } catch (err) {
      setErrorMsg('Connection error.');
    }
  };

  // Helper: Open Secret Modal for Edit
  const openEditModal = (key: string, val: string) => {
    setSecretKey(key);
    setSecretValue(val);
    setIsEditingSecret(true);
    setShowSecretModal(true);
  };

  // Helper: Open Secret Modal for Add
  const openAddModal = () => {
    setSecretKey('');
    setSecretValue('');
    setIsEditingSecret(false);
    setShowSecretModal(true);
  };

  // Copy to clipboard helper
  const copyToClipboard = (key: string, value: string) => {
    navigator.clipboard.writeText(value);
    setCopiedKey(key);
    setTimeout(() => setCopiedKey(null), 2000);
  };

  // Toggle Project Expanded
  const toggleProjectExpand = (projName: string) => {
    setExpandedProjects(prev => ({
      ...prev,
      [projName]: !prev[projName]
    }));
  };

  // Toggle visibility of individual secret
  const toggleSecretVisibility = (key: string) => {
    setVisibleSecrets(prev => ({
      ...prev,
      [key]: !prev[key]
    }));
  };

  // Filtered Secrets
  const filteredSecrets = useMemo(() => {
    return Object.entries(secrets).filter(([key, val]) => {
      const query = searchQuery.toLowerCase();
      return key.toLowerCase().includes(query) || val.toLowerCase().includes(query);
    });
  }, [secrets, searchQuery]);

  return (
    <div className="app-container">
      {/* Toast Messages */}
      {errorMsg && (
        <div className="toast error" style={{
          position: 'fixed', top: '24px', right: '24px', background: 'rgba(239, 68, 68, 0.9)',
          color: 'white', padding: '1rem 1.5rem', borderRadius: '10px', backdropFilter: 'blur(8px)',
          zIndex: 1000, boxShadow: '0 10px 30px rgba(0,0,0,0.5)', border: '1px solid rgba(255,255,255,0.1)'
        }}>
          <strong>Error:</strong> {errorMsg}
        </div>
      )}

      {successMsg && (
        <div className="toast success" style={{
          position: 'fixed', top: '24px', right: '24px', background: 'rgba(16, 185, 129, 0.9)',
          color: 'white', padding: '1rem 1.5rem', borderRadius: '10px', backdropFilter: 'blur(8px)',
          zIndex: 1000, boxShadow: '0 10px 30px rgba(0,0,0,0.5)', border: '1px solid rgba(255,255,255,0.1)'
        }}>
          {successMsg}
        </div>
      )}

      {/* Header */}
      <header className="app-header glass-panel">
        <div className="brand">
          <Shield className="brand-icon" />
          <h1>shroudenv</h1>
        </div>
        <div style={{ display: 'flex', alignItems: 'center', gap: '1rem' }}>
          {statusInfo && (
            <div className="status-badge">
              <Activity size={14} />
              <span>Vault Active</span>
            </div>
          )}
          {!token && (
            <div className="status-badge disconnected">
              <Lock size={14} />
              <span>Locked</span>
            </div>
          )}
        </div>
      </header>

      {/* Main Grid */}
      <div className="dashboard-grid">
        {/* Sidebar */}
        <aside className="sidebar glass-panel">
          <div>
            <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginBottom: '0.75rem' }}>
              <span className="section-title">Projects</span>
              <button
                className="btn-icon"
                onClick={() => setShowProjModal(true)}
                title="New Project"
                id="btn-new-project"
              >
                <FolderPlus size={18} />
              </button>
            </div>

            <div className="project-list">
              {projects.map(proj => (
                <div key={proj.name} className="project-item">
                  <div
                    className={`project-header ${selectedProject === proj.name ? 'active' : ''}`}
                    onClick={() => {
                      setSelectedProject(proj.name);
                      toggleProjectExpand(proj.name);
                      if (proj.environments.length > 0) {
                        setSelectedEnv(proj.environments[0].name);
                      } else {
                        setSelectedEnv('');
                      }
                    }}
                  >
                    <span>{proj.name}</span>
                    <span style={{ fontSize: '0.8rem', opacity: 0.6 }}>
                      {expandedProjects[proj.name] ? '▼' : '►'}
                    </span>
                  </div>

                  {expandedProjects[proj.name] && (
                    <div className="env-list">
                      {proj.environments.map(env => (
                        <div
                          key={env.name}
                          className={`env-item ${selectedProject === proj.name && selectedEnv === env.name ? 'active' : ''}`}
                          onClick={(e) => {
                            e.stopPropagation();
                            setSelectedProject(proj.name);
                            setSelectedEnv(env.name);
                          }}
                        >
                          <span>{env.name}</span>
                          <span className={`env-status-dot ${env.has_secrets ? 'has-secrets' : ''}`} />
                        </div>
                      ))}

                      <div
                        className="env-item"
                        style={{ fontStyle: 'italic', justifyContent: 'center', opacity: 0.6 }}
                        onClick={(e) => {
                          e.stopPropagation();
                          setSelectedProject(proj.name);
                          setShowEnvModal(true);
                        }}
                      >
                        <Plus size={14} style={{ marginRight: '4px' }} /> Add Env
                      </div>
                    </div>
                  )}
                </div>
              ))}

              {projects.length === 0 && (
                <div style={{ padding: '1rem', textAlign: 'center', color: 'var(--text-muted)' }}>
                  No projects created yet.
                </div>
              )}
            </div>
          </div>

          <div style={{ marginTop: 'auto', borderTop: '1px solid var(--border-color)', paddingTop: '1.25rem' }}>
            <span className="section-title">Database</span>
            <div style={{ fontSize: '0.8rem', color: 'var(--text-secondary)', wordBreak: 'break-all', display: 'flex', flexDirection: 'column', gap: '4px', marginTop: '0.5rem' }}>
              <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
                <div style={{ fontWeight: 600 }}>Local Storage Path:</div>
                <button
                  type="button"
                  className="btn-icon"
                  style={{ padding: '2px', minWidth: 'auto', height: 'auto', cursor: 'pointer' }}
                  onClick={() => setShowDbPath(!showDbPath)}
                  title={showDbPath ? "Mask path" : "Reveal path"}
                >
                  {showDbPath ? <EyeOff size={14} /> : <Eye size={14} />}
                </button>
              </div>
              <div style={{ color: 'var(--text-muted)', fontFamily: 'var(--font-mono)' }}>
                {statusInfo?.db_path 
                  ? (showDbPath ? statusInfo.db_path : '••••••••••••••••') 
                  : 'Retrieving...'}
              </div>
            </div>
          </div>
        </aside>

        {/* Main Content Area */}
        <main className="main-content glass-panel">
          {selectedProject && selectedEnv ? (
            <>
              {/* Content Header */}
              <div className="content-header">
                <div className="selected-info">
                  <h2>
                    <span>{selectedProject}</span>
                    <span style={{ color: 'var(--text-muted)', fontWeight: 400 }}>/</span>
                    <span style={{ color: 'var(--accent-primary)' }}>{selectedEnv}</span>
                  </h2>
                </div>

                <div style={{ display: 'flex', gap: '0.75rem' }}>
                  <button
                    className="btn btn-secondary"
                    onClick={() => {
                      setImportText('');
                      setImportTab('upload');
                      setImportStrategy('merge');
                      setShowImportModal(true);
                    }}
                    id="btn-import-env"
                  >
                    <FolderPlus size={16} /> Import .env
                  </button>
                  <button
                    className="btn btn-secondary"
                    onClick={() => setShowBootstrapModal(true)}
                    id="btn-bootstrap"
                    disabled={Object.keys(secrets).length > 0}
                    title={Object.keys(secrets).length > 0 ? "Bootstrapping is only allowed on empty environments" : "Bootstrap from .shroudenv.yaml"}
                  >
                    <Sparkles size={16} /> Bootstrap
                  </button>
                  <button
                    className="btn btn-primary"
                    onClick={openAddModal}
                    id="btn-add-secret"
                  >
                    <Plus size={16} /> Add Secret
                  </button>
                </div>
              </div>

              {/* Controls */}
              <div className="secrets-controls">
                <div className="search-box">
                  <Search className="search-icon" />
                  <input
                    type="text"
                    placeholder="Search secrets by key or value..."
                    className="search-input"
                    value={searchQuery}
                    onChange={(e) => setSearchQuery(e.target.value)}
                    id="search-secrets"
                  />
                </div>

                <button
                  className="btn btn-secondary"
                  onClick={() => {
                    setShowAllSecrets(!showAllSecrets);
                    const allVisible = !showAllSecrets;
                    const nextVisible: Record<string, boolean> = {};
                    Object.keys(secrets).forEach(k => {
                      nextVisible[k] = allVisible;
                    });
                    setVisibleSecrets(nextVisible);
                  }}
                >
                  {showAllSecrets ? <EyeOff size={16} /> : <Eye size={16} />}
                  <span>{showAllSecrets ? 'Hide All' : 'Show All'}</span>
                </button>
              </div>

              {/* Secrets Table */}
              {filteredSecrets.length > 0 ? (
                <div className="secrets-table-container">
                  <table className="secrets-table">
                    <thead>
                      <tr>
                        <th>Key</th>
                        <th>Value</th>
                        <th style={{ width: '160px', textAlign: 'right' }}>Actions</th>
                      </tr>
                    </thead>
                    <tbody>
                      {filteredSecrets.map(([key, value]) => {
                        const isVisible = visibleSecrets[key];
                        return (
                          <tr key={key}>
                            <td className="secret-key-cell">{key}</td>
                            <td className="secret-value-cell">
                              {isVisible ? (
                                <span className="secret-value-text">{value}</span>
                              ) : (
                                <span className="secret-value-masked">••••••••••••••••</span>
                              )}
                            </td>
                            <td>
                              <div className="action-buttons">
                                <button
                                  className="btn-icon"
                                  onClick={() => toggleSecretVisibility(key)}
                                  title={isVisible ? "Hide value" : "Show value"}
                                >
                                  {isVisible ? <EyeOff size={16} /> : <Eye size={16} />}
                                </button>
                                <button
                                  className="btn-icon"
                                  onClick={() => copyToClipboard(key, value)}
                                  title="Copy value"
                                >
                                  {copiedKey === key ? <Check size={16} style={{ color: 'var(--accent-success)' }} /> : <Copy size={16} />}
                                </button>
                                <button
                                  className="btn-icon"
                                  onClick={() => openEditModal(key, value)}
                                  title="Edit secret"
                                >
                                  <Key size={16} />
                                </button>
                                <button
                                  className="btn-icon btn-danger"
                                  onClick={() => handleDeleteSecret(key)}
                                  title="Delete secret"
                                >
                                  <Trash2 size={16} />
                                </button>
                              </div>
                            </td>
                          </tr>
                        );
                      })}
                    </tbody>
                  </table>
                </div>
              ) : (
                <div className="empty-state">
                  <SearchCode className="empty-state-icon" />
                  <h3>No Secrets Found</h3>
                  {searchQuery ? (
                    <p>No secrets matched your search query "{searchQuery}"</p>
                  ) : (
                    <p>Click "Add Secret" to secure environment credentials inside this environment.</p>
                  )}
                </div>
              )}
            </>
          ) : (
            <div className="empty-state" style={{ height: '100%', minHeight: '400px' }}>
              <Lock className="empty-state-icon" style={{ color: 'var(--accent-primary)', opacity: 0.5 }} />
              <h3>Select a Project and Environment</h3>
              <p>Choose an environment from the sidebar, or create a new project to start managing secure environments.</p>
              <button
                className="btn btn-secondary"
                style={{ marginTop: '1rem' }}
                onClick={() => setShowProjModal(true)}
              >
                <Plus size={16} /> Create Project
              </button>
            </div>
          )}
        </main>
      </div>

      {/* Project Modal */}
      {showProjModal && (
        <div className="modal-overlay">
          <div className="modal-content glass-panel">
            <div className="modal-header" style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
              <h3>Create Project</h3>
              <button className="btn-icon" onClick={() => setShowProjModal(false)}><X size={18} /></button>
            </div>
            <form onSubmit={handleCreateProject}>
              <div className="form-group">
                <label htmlFor="proj-name">Project Name</label>
                <input
                  type="text"
                  id="proj-name"
                  className="form-control"
                  placeholder="e.g. ecommerce-api"
                  value={newProjName}
                  onChange={(e) => setNewProjName(e.target.value)}
                  autoFocus
                />
              </div>
              <div className="modal-footer">
                <button type="button" className="btn btn-secondary" onClick={() => setShowProjModal(false)}>Cancel</button>
                <button type="submit" className="btn btn-primary">Create</button>
              </div>
            </form>
          </div>
        </div>
      )}

      {/* Environment Modal */}
      {showEnvModal && (
        <div className="modal-overlay">
          <div className="modal-content glass-panel">
            <div className="modal-header" style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
              <h3>Create Environment</h3>
              <button className="btn-icon" onClick={() => setShowEnvModal(false)}><X size={18} /></button>
            </div>
            <form onSubmit={handleCreateEnv}>
              <div style={{ marginBottom: '1rem', fontSize: '0.9rem', color: 'var(--text-secondary)' }}>
                Creating environment inside project <strong>{selectedProject}</strong>.
              </div>
              <div className="form-group">
                <label htmlFor="env-name">Environment Name</label>
                <input
                  type="text"
                  id="env-name"
                  className="form-control"
                  placeholder="e.g. development, production"
                  value={newEnvName}
                  onChange={(e) => setNewEnvName(e.target.value)}
                  autoFocus
                />
              </div>
              <div className="modal-footer">
                <button type="button" className="btn btn-secondary" onClick={() => setShowEnvModal(false)}>Cancel</button>
                <button type="submit" className="btn btn-primary">Create</button>
              </div>
            </form>
          </div>
        </div>
      )}

      {/* Secret Modal */}
      {showSecretModal && (
        <div className="modal-overlay">
          <div className="modal-content glass-panel">
            <div className="modal-header" style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
              <h3>{isEditingSecret ? 'Edit Secret' : 'Add Secret'}</h3>
              <button className="btn-icon" onClick={() => setShowSecretModal(false)}><X size={18} /></button>
            </div>
            <form onSubmit={handleSetSecret}>
              <div style={{ display: 'flex', flexDirection: 'column', gap: '1rem' }}>
                <div className="form-group">
                  <label htmlFor="sec-key">Key / Variable Name</label>
                  <input
                    type="text"
                    id="sec-key"
                    className="form-control code"
                    placeholder="e.g. DATABASE_URL"
                    value={secretKey}
                    onChange={(e) => setSecretKey(e.target.value)}
                    disabled={isEditingSecret}
                    autoFocus={!isEditingSecret}
                  />
                </div>
                <div className="form-group">
                  <label htmlFor="sec-val">Value</label>
                  <textarea
                    id="sec-val"
                    className="form-control code"
                    rows={4}
                    placeholder="Enter secret value..."
                    value={secretValue}
                    onChange={(e) => setSecretValue(e.target.value)}
                    autoFocus={isEditingSecret}
                  />
                </div>
              </div>
              <div className="modal-footer">
                <button type="button" className="btn btn-secondary" onClick={() => setShowSecretModal(false)}>Cancel</button>
                <button type="submit" className="btn btn-primary">{isEditingSecret ? 'Update' : 'Save'}</button>
              </div>
            </form>
          </div>
        </div>
      )}

      {/* Import .env Modal */}
      {showImportModal && (
        <div className="modal-overlay">
          <div className="modal-content glass-panel" style={{ maxWidth: '600px' }}>
            <div className="modal-header" style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
              <h3>Import .env File</h3>
              <button className="btn-icon" onClick={() => setShowImportModal(false)}><X size={18} /></button>
            </div>

            <div className="tab-buttons">
              <button
                type="button"
                className={`tab-button ${importTab === 'upload' ? 'active' : ''}`}
                onClick={() => setImportTab('upload')}
              >
                Upload File
              </button>
              <button
                type="button"
                className={`tab-button ${importTab === 'paste' ? 'active' : ''}`}
                onClick={() => setImportTab('paste')}
              >
                Paste Content
              </button>
            </div>

            <form onSubmit={handleConfirmImport} style={{ display: 'flex', flexDirection: 'column', gap: '1.25rem' }}>
              {importTab === 'upload' ? (
                <div
                  className={`drag-drop-zone ${dragActive ? 'active' : ''}`}
                  onDragEnter={handleDrag}
                  onDragOver={handleDrag}
                  onDragLeave={handleDrag}
                  onDrop={handleDrop}
                  onClick={() => document.getElementById('env-file-input')?.click()}
                >
                  <FolderPlus className="drag-drop-icon" />
                  <div>
                    <p style={{ fontWeight: 600 }}>Drag and drop your .env file here</p>
                    <p style={{ fontSize: '0.8rem', color: 'var(--text-secondary)', marginTop: '0.25rem' }}>Or click to select from your files</p>
                  </div>
                  <input
                    type="file"
                    id="env-file-input"
                    accept=".env,text/plain"
                    style={{ display: 'none' }}
                    onChange={handleFileChange}
                  />
                </div>
              ) : (
                <div className="form-group">
                  <label htmlFor="import-textarea">Pasted .env Content</label>
                  <textarea
                    id="import-textarea"
                    className="form-control code"
                    rows={6}
                    placeholder="PORT=8080&#10;DATABASE_URL=mongodb://localhost:27017&#10;# comment lines are ignored"
                    value={importText}
                    onChange={(e) => setImportText(e.target.value)}
                    autoFocus
                  />
                </div>
              )}

              {/* Parsed Preview Section */}
              {Object.keys(parsedImportSecrets).length > 0 && (
                <div className="preview-section">
                  <div className="preview-header">
                    <span>Parsed Preview</span>
                    <span>{Object.keys(parsedImportSecrets).length} secrets detected</span>
                  </div>
                  <div className="preview-scroll-container">
                    {Object.entries(parsedImportSecrets).map(([k, v]) => (
                      <div key={k} className="preview-row">
                        <span className="preview-row-key">{k}</span>
                        <span className="preview-row-value">{v}</span>
                      </div>
                    ))}
                  </div>
                </div>
              )}

              {/* Import Strategy Selection */}
              {Object.keys(parsedImportSecrets).length > 0 && (
                <div className="strategy-selection">
                  <label style={{ fontSize: '0.85rem', fontWeight: 600, color: 'var(--text-secondary)' }}>Import Strategy</label>
                  <div className="strategy-options">
                    <label className="strategy-label">
                      <input
                        type="radio"
                        name="importStrategy"
                        value="merge"
                        checked={importStrategy === 'merge'}
                        onChange={() => setImportStrategy('merge')}
                      />
                      <span>Merge (preserves other variables)</span>
                    </label>
                    <label className="strategy-label">
                      <input
                        type="radio"
                        name="importStrategy"
                        value="overwrite"
                        checked={importStrategy === 'overwrite'}
                        onChange={() => setImportStrategy('overwrite')}
                      />
                      <span>Overwrite (replaces all variables)</span>
                    </label>
                  </div>
                </div>
              )}

              <div className="modal-footer">
                <button type="button" className="btn btn-secondary" onClick={() => setShowImportModal(false)}>Cancel</button>
                <button
                  type="submit"
                  className="btn btn-primary"
                  disabled={Object.keys(parsedImportSecrets).length === 0}
                >
                  Confirm Import
                </button>
              </div>
            </form>
          </div>
        </div>
      )}

      <BootstrapModal
        isOpen={showBootstrapModal}
        onClose={() => setShowBootstrapModal(false)}
        projectName={selectedProject}
        envName={selectedEnv}
        token={token}
        onSuccess={() => {
          setShowBootstrapModal(false);
          setSuccessMsg('Environment bootstrapped successfully.');
          fetchSecrets();
          fetchStatusAndProjects();
        }}
      />
    </div>
  );
}

export default App;
