import React, { useState, useEffect } from 'react';
import {
  X,
  Sparkles,
  Eye,
  EyeOff,
  Activity,
  FileCode
} from 'lucide-react';

interface ValidationSchema {
  min?: number;
  max?: number;
  enum?: string[];
  pattern?: string;
  error_message?: string;
}

interface ResolvedVariable {
  name: string;
  description?: string;
  type?: string;
  prompt?: string;
  sensitive?: boolean;
  optional?: boolean;
  validation?: ValidationSchema;
  pre_resolved_value: string;
  is_generated: boolean;
}

interface BootstrapModalProps {
  isOpen: boolean;
  onClose: () => void;
  projectName: string;
  envName: string;
  token: string;
  onSuccess: () => void;
}

const BACKEND_URL = import.meta.env.DEV ? 'http://localhost:4554' : '';

export function BootstrapModal({
  isOpen,
  onClose,
  projectName,
  envName,
  token,
  onSuccess
}: BootstrapModalProps) {
  const [step, setStep] = useState<'config' | 'form'>('config');
  const [importTab, setImportTab] = useState<'upload' | 'paste'>('upload');
  const [yamlText, setYamlText] = useState<string>('');
  const [dragActive, setDragActive] = useState<boolean>(false);
  const [loading, setLoading] = useState<boolean>(false);
  const [errorMessage, setErrorMessage] = useState<string>('');

  // Parsed configuration details
  const [scaffoldProject, setScaffoldProject] = useState<string>('');
  const [variables, setVariables] = useState<ResolvedVariable[]>([]);
  const [inputs, setInputs] = useState<Record<string, string>>({});
  const [validationErrors, setValidationErrors] = useState<Record<string, string>>({});
  const [visibleSecrets, setVisibleSecrets] = useState<Record<string, boolean>>({});

  // Reset modal state on open/close
  useEffect(() => {
    if (isOpen) {
      setStep('config');
      setImportTab('upload');
      setYamlText('');
      setErrorMessage('');
      setLoading(false);
      setScaffoldProject('');
      setVariables([]);
      setInputs({});
      setValidationErrors({});
      setVisibleSecrets({});
    }
  }, [isOpen]);

  // Debounce API validation call to avoid overloading the backend
  useEffect(() => {
    if (step !== 'form' || !yamlText.trim()) return;

    const timer = setTimeout(async () => {
      try {
        const res = await fetch(`${BACKEND_URL}/api/bootstrap/validate`, {
          method: 'POST',
          headers: {
            'Content-Type': 'application/json',
            'Authorization': `Bearer ${token}`
          },
          body: JSON.stringify({ yaml: yamlText, inputs })
        });
        if (res.ok) {
          const data = await res.json();
          setValidationErrors(data.errors || {});
        }
      } catch (err) {
        // Ignore network errors during typing
      }
    }, 150);

    return () => clearTimeout(timer);
  }, [inputs, yamlText, step, token]);

  const handleInputChange = (varName: string, value: string) => {
    setInputs(prev => ({ ...prev, [varName]: value }));
  };

  const handleDrag = (e: React.DragEvent) => {
    e.preventDefault();
    e.stopPropagation();
    if (e.type === 'dragenter' || e.type === 'dragover') {
      setDragActive(true);
    } else if (e.type === 'dragleave') {
      setDragActive(false);
    }
  };

  const handleDrop = (e: React.DragEvent) => {
    e.preventDefault();
    e.stopPropagation();
    setDragActive(false);

    const file = e.dataTransfer.files?.[0];
    if (file) {
      readFile(file);
    }
  };

  const handleFileChange = (e: React.ChangeEvent<HTMLInputElement>) => {
    const file = e.target.files?.[0];
    if (file) {
      readFile(file);
    }
  };

  const readFile = (file: File) => {
    const reader = new FileReader();
    reader.onload = (event) => {
      const text = event.target?.result as string;
      setYamlText(text);
      setImportTab('paste'); // Switch to editor tab to let them see it
    };
    reader.readAsText(file);
  };

  const handleParseConfig = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!yamlText.trim()) return;

    setLoading(true);
    setErrorMessage('');

    try {
      const res = await fetch(`${BACKEND_URL}/api/bootstrap/parse`, {
        method: 'POST',
        headers: {
          'Content-Type': 'application/json',
          'Authorization': `Bearer ${token}`
        },
        body: JSON.stringify({ yaml: yamlText })
      });

      if (res.ok) {
        const data = await res.json();
        setScaffoldProject(data.project);
        setVariables(data.variables || []);

        // Initialize inputs with pre-resolved values
        const initialInputs: Record<string, string> = {};
        const parsedVars: ResolvedVariable[] = data.variables || [];

        parsedVars.forEach(v => {
          initialInputs[v.name] = v.pre_resolved_value || '';
        });

        setInputs(initialInputs);
        setValidationErrors({});
        setStep('form');
      } else {
        const errorText = await res.text();
        setErrorMessage(errorText || 'Failed to parse configuration file.');
      }
    } catch (err) {
      setErrorMessage('Failed to connect to backend server.');
    } finally {
      setLoading(false);
    }
  };

  const handleConfirmBootstrap = async (e: React.FormEvent) => {
    e.preventDefault();

    // Prevent submit if validation errors exist in local state
    const hasErrors = Object.values(validationErrors).some(err => err !== '');
    if (hasErrors) return;

    setLoading(true);
    setErrorMessage('');

    try {
      // 1. Perform final validation check on the backend
      const valRes = await fetch(`${BACKEND_URL}/api/bootstrap/validate`, {
        method: 'POST',
        headers: {
          'Content-Type': 'application/json',
          'Authorization': `Bearer ${token}`
        },
        body: JSON.stringify({ yaml: yamlText, inputs })
      });

      if (valRes.ok) {
        const valData = await valRes.json();
        if (!valData.valid) {
          setValidationErrors(valData.errors || {});
          setErrorMessage('Validation failed. Please correct the highlighted inputs.');
          setLoading(false);
          return;
        }
      }

      // 2. Submit to bootstrap endpoint
      const res = await fetch(`${BACKEND_URL}/api/projects/${encodeURIComponent(projectName)}/envs/${encodeURIComponent(envName)}/bootstrap`, {
        method: 'POST',
        headers: {
          'Content-Type': 'application/json',
          'Authorization': `Bearer ${token}`
        },
        body: JSON.stringify({ secrets: inputs })
      });

      if (res.ok) {
        onSuccess();
      } else {
        const errorText = await res.text();
        setErrorMessage(errorText || 'Failed to bootstrap environment.');
      }
    } catch (err) {
      setErrorMessage('Network error during bootstrapping.');
    } finally {
      setLoading(false);
    }
  };

  const toggleSecretVisibility = (name: string) => {
    setVisibleSecrets(prev => ({ ...prev, [name]: !prev[name] }));
  };

  const hasFormErrors = Object.values(validationErrors).some(err => err !== '');

  if (!isOpen) return null;

  return (
    <div className="modal-overlay">
      <div className="modal-content glass-panel" style={{ maxWidth: '680px', width: '90%' }}>
        <div className="modal-header" style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginBottom: '1.25rem' }}>
          <div style={{ display: 'flex', alignItems: 'center', gap: '0.5rem' }}>
            <Sparkles className="brand-icon" style={{ color: 'var(--accent-primary)', width: '20px', height: '20px' }} />
            <h3>Bootstrap Environment</h3>
          </div>
          <button className="btn-icon" onClick={onClose}><X size={18} /></button>
        </div>

        {errorMessage && (
          <div style={{
            background: 'rgba(239, 68, 68, 0.15)',
            border: '1px solid rgba(239, 68, 68, 0.3)',
            color: '#f87171',
            padding: '0.75rem 1rem',
            borderRadius: '8px',
            fontSize: '0.875rem',
            marginBottom: '1rem',
            lineHeight: 1.4
          }}>
            <strong>Error:</strong> {errorMessage}
          </div>
        )}

        {step === 'config' ? (
          <div>
            <div style={{ marginBottom: '1.25rem', fontSize: '0.9rem', color: 'var(--text-secondary)' }}>
              Scaffold environment variables inside <strong>{projectName}</strong> / <strong style={{ color: 'var(--accent-primary)' }}>{envName}</strong> by uploading a <code>.shroudenv.yaml</code> configuration file.
            </div>

            <div className="tab-buttons">
              <button
                type="button"
                className={`tab-button ${importTab === 'upload' ? 'active' : ''}`}
                onClick={() => setImportTab('upload')}
              >
                Upload Config
              </button>
              <button
                type="button"
                className={`tab-button ${importTab === 'paste' ? 'active' : ''}`}
                onClick={() => setImportTab('paste')}
              >
                Paste Content
              </button>
            </div>

            <form onSubmit={handleParseConfig} style={{ display: 'flex', flexDirection: 'column', gap: '1.25rem' }}>
              {importTab === 'upload' ? (
                <div
                  className={`drag-drop-zone ${dragActive ? 'active' : ''}`}
                  onDragEnter={handleDrag}
                  onDragOver={handleDrag}
                  onDragLeave={handleDrag}
                  onDrop={handleDrop}
                  onClick={() => document.getElementById('shroudenv-yaml-input')?.click()}
                >
                  <FileCode className="drag-drop-icon" style={{ color: 'var(--accent-primary)' }} />
                  <div>
                    <p style={{ fontWeight: 600 }}>Drag and drop your .shroudenv.yaml file here</p>
                    <p style={{ fontSize: '0.8rem', color: 'var(--text-secondary)', marginTop: '0.25rem' }}>Or click to select from files</p>
                  </div>
                  <input
                    type="file"
                    id="shroudenv-yaml-input"
                    accept=".yaml,.yml,text/yaml"
                    style={{ display: 'none' }}
                    onChange={handleFileChange}
                  />
                </div>
              ) : (
                <div className="form-group">
                  <label htmlFor="bootstrap-yaml-textarea">YAML Configuration</label>
                  <textarea
                    id="bootstrap-yaml-textarea"
                    className="form-control code"
                    rows={8}
                    placeholder={`version: "1"\nproject: "my-app"\nvariables:\n  - name: PORT\n    type: integer\n    default: 3000`}
                    value={yamlText}
                    onChange={(e) => setYamlText(e.target.value)}
                    autoFocus
                  />
                </div>
              )}

              <div className="modal-footer" style={{ marginTop: '0.5rem' }}>
                <button type="button" className="btn btn-secondary" onClick={onClose}>Cancel</button>
                <button
                  type="submit"
                  className="btn btn-primary"
                  disabled={loading || !yamlText.trim()}
                >
                  {loading ? 'Parsing...' : 'Analyze Configuration'}
                </button>
              </div>
            </form>
          </div>
        ) : (
          <form onSubmit={handleConfirmBootstrap} style={{ display: 'flex', flexDirection: 'column', gap: '1.25rem' }}>
            {scaffoldProject && scaffoldProject !== projectName && (
              <div style={{
                background: 'rgba(245, 158, 11, 0.15)',
                border: '1px solid rgba(245, 158, 11, 0.3)',
                color: '#fbbf24',
                padding: '0.75rem 1rem',
                borderRadius: '8px',
                fontSize: '0.875rem',
                display: 'flex',
                alignItems: 'center',
                gap: '0.5rem'
              }}>
                <Activity size={18} />
                <span>
                  The scaffolding template specifies project <strong>"{scaffoldProject}"</strong> but you are applying it to <strong>"{projectName}"</strong>.
                </span>
              </div>
            )}

            <div style={{ maxHeight: '380px', overflowY: 'auto', paddingRight: '0.5rem', display: 'flex', flexDirection: 'column', gap: '1rem' }}>
              {variables.map(v => {
                const isSensitive = v.sensitive;
                const isVisible = visibleSecrets[v.name];
                const hasError = !!validationErrors[v.name];
                const error = validationErrors[v.name];
                const inputVal = inputs[v.name] || '';

                return (
                  <div key={v.name} className="form-group" style={{ background: 'rgba(255,255,255,0.02)', padding: '0.75rem 1rem', borderRadius: '8px', border: '1px solid var(--border-color)' }}>
                    <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginBottom: '0.375rem' }}>
                      <div style={{ display: 'flex', alignItems: 'center', gap: '0.5rem' }}>
                        <span style={{ fontFamily: 'var(--font-mono)', fontWeight: 600, fontSize: '0.95rem' }}>{v.name}</span>
                        {!v.optional && <span style={{ color: '#ef4444', fontSize: '0.75rem', fontWeight: 600 }}>* Required</span>}
                      </div>
                      <div style={{ display: 'flex', gap: '0.375rem', alignItems: 'center' }}>
                        <span style={{ fontSize: '0.7rem', color: 'var(--text-muted)', border: '1px solid var(--border-color)', padding: '1px 6px', borderRadius: '4px', textTransform: 'lowercase' }}>
                          {v.type || 'string'}
                        </span>
                        {v.is_generated && (
                          <span style={{
                            fontSize: '0.7rem',
                            fontWeight: 600,
                            background: 'linear-gradient(135deg, #a855f7 0%, #3b82f6 100%)',
                            color: 'white',
                            padding: '1px 6px',
                            borderRadius: '4px',
                            display: 'flex',
                            alignItems: 'center',
                            gap: '2px'
                          }}>
                            ✨ Generated
                          </span>
                        )}
                      </div>
                    </div>

                    {v.description && (
                      <div style={{ fontSize: '0.8rem', color: 'var(--text-secondary)', marginBottom: '0.5rem' }}>
                        {v.description}
                      </div>
                    )}

                    <div style={{ display: 'flex', gap: '0.5rem', position: 'relative' }}>
                      {v.validation?.enum && v.validation.enum.length > 0 ? (
                        <select
                          className="form-control"
                          value={inputVal}
                          onChange={(e) => handleInputChange(v.name, e.target.value)}
                          style={{ fontFamily: 'var(--font-mono)' }}
                        >
                          {v.optional && <option value="">-- Skip Optional --</option>}
                          {v.validation.enum.map(opt => (
                            <option key={opt} value={opt}>{opt}</option>
                          ))}
                        </select>
                      ) : (
                        <div style={{ flex: 1, display: 'flex', position: 'relative' }}>
                          <input
                            type={isSensitive && !isVisible ? 'password' : 'text'}
                            className={`form-control code ${hasError ? 'invalid' : ''}`}
                            value={inputVal}
                            onChange={(e) => handleInputChange(v.name, e.target.value)}
                            placeholder={v.prompt || `Enter value for ${v.name}...`}
                            style={{
                              paddingRight: isSensitive ? '40px' : '10px',
                              border: hasError ? '1px solid #f87171' : undefined
                            }}
                          />
                          {isSensitive && (
                            <button
                              type="button"
                              className="btn-icon"
                              style={{
                                position: 'absolute',
                                right: '10px',
                                top: '50%',
                                transform: 'translateY(-50%)',
                                padding: 0,
                                minWidth: 'auto',
                                height: 'auto',
                                background: 'none'
                              }}
                              onClick={() => toggleSecretVisibility(v.name)}
                              title={isVisible ? 'Hide value' : 'Show value'}
                            >
                              {isVisible ? <EyeOff size={16} /> : <Eye size={16} />}
                            </button>
                          )}
                        </div>
                      )}
                    </div>

                    {hasError && (
                      <div style={{ color: '#f87171', fontSize: '0.75rem', marginTop: '0.375rem', fontWeight: 500 }}>
                        {error}
                      </div>
                    )}
                  </div>
                );
              })}
            </div>

            <div className="modal-footer" style={{ marginTop: '0.5rem', borderTop: '1px solid var(--border-color)', paddingTop: '1rem' }}>
              <button type="button" className="btn btn-secondary" onClick={() => setStep('config')}>Back</button>
              <button
                type="submit"
                className="btn btn-primary"
                disabled={loading || hasFormErrors}
              >
                {loading ? 'Bootstrapping...' : 'Confirm Bootstrap'}
              </button>
            </div>
          </form>
        )}
      </div>
    </div>
  );
}
