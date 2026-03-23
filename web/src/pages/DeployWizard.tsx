import { useState } from 'react';
import { useNavigate } from 'react-router';
import { Rocket, GitBranch, Container, Store, ArrowRight, ArrowLeft, Check } from 'lucide-react';
import { appsAPI } from '../api/apps';

type SourceType = 'git' | 'image' | 'marketplace';

const steps = ['Source', 'Configure', 'Deploy'];

export function DeployWizard() {
  const navigate = useNavigate();
  const [step, setStep] = useState(0);
  const [sourceType, setSourceType] = useState<SourceType | null>(null);
  const [config, setConfig] = useState({
    name: '',
    sourceURL: '',
    branch: 'main',
    port: '3000',
  });
  const [deploying, setDeploying] = useState(false);
  const [error, setError] = useState('');

  const handleDeploy = async () => {
    setError('');
    setDeploying(true);
    try {
      const app = await appsAPI.create({
        name: config.name,
        source_type: sourceType || 'image',
        source_url: config.sourceURL,
        branch: config.branch,
      });
      navigate(`/apps/${app.id}`);
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Deploy failed');
      setDeploying(false);
    }
  };

  return (
    <div className="max-w-2xl mx-auto space-y-8">
      <div>
        <h1 className="text-2xl font-semibold text-text-primary">Deploy New Application</h1>
        <p className="text-sm text-text-secondary mt-1">Follow the steps to deploy your application</p>
      </div>

      {/* Progress */}
      <div className="flex items-center justify-between">
        {steps.map((label, i) => (
          <div key={label} className="flex items-center gap-2">
            <div className={`w-8 h-8 rounded-full flex items-center justify-center text-sm font-medium ${
              i < step ? 'bg-monster-green text-white' :
              i === step ? 'bg-monster-green/20 text-monster-green border-2 border-monster-green' :
              'bg-surface-tertiary text-text-muted'
            }`}>
              {i < step ? <Check size={16} /> : i + 1}
            </div>
            <span className={`text-sm ${i <= step ? 'text-text-primary' : 'text-text-muted'}`}>{label}</span>
            {i < steps.length - 1 && <div className="w-16 h-px bg-border mx-2" />}
          </div>
        ))}
      </div>

      {/* Step 1: Source */}
      {step === 0 && (
        <div className="space-y-4">
          <h2 className="font-medium text-text-primary">Choose deployment source</h2>
          <div className="grid grid-cols-1 sm:grid-cols-3 gap-3">
            {([
              { type: 'git' as SourceType, icon: GitBranch, label: 'Git Repository', desc: 'Deploy from GitHub, GitLab, etc.' },
              { type: 'image' as SourceType, icon: Container, label: 'Docker Image', desc: 'Deploy a pre-built image' },
              { type: 'marketplace' as SourceType, icon: Store, label: 'Marketplace', desc: 'One-click app template' },
            ]).map(({ type, icon: Icon, label, desc }) => (
              <button key={type} onClick={() => setSourceType(type)}
                className={`p-4 rounded-xl border text-left transition-colors ${
                  sourceType === type
                    ? 'border-monster-green bg-monster-green/5'
                    : 'border-border hover:border-monster-green/30'
                }`}>
                <Icon size={24} className={sourceType === type ? 'text-monster-green' : 'text-text-muted'} />
                <p className="font-medium text-text-primary mt-2">{label}</p>
                <p className="text-xs text-text-secondary mt-1">{desc}</p>
              </button>
            ))}
          </div>
          <div className="flex justify-end">
            <button onClick={() => sourceType && setStep(1)} disabled={!sourceType}
              className="flex items-center gap-2 px-4 py-2 bg-monster-green hover:bg-monster-green-dark text-white text-sm rounded-lg disabled:opacity-50 transition-colors">
              Next <ArrowRight size={16} />
            </button>
          </div>
        </div>
      )}

      {/* Step 2: Configure */}
      {step === 1 && (
        <div className="space-y-4">
          <h2 className="font-medium text-text-primary">Configure your application</h2>
          <div className="bg-surface border border-border rounded-xl p-6 space-y-4">
            <div>
              <label className="block text-sm font-medium text-text-secondary mb-1">Application Name</label>
              <input type="text" value={config.name} onChange={(e) => setConfig({ ...config, name: e.target.value })}
                className="w-full px-3 py-2 rounded-lg border border-border bg-surface text-text-primary focus:ring-2 focus:ring-monster-green/50"
                placeholder="my-awesome-app" />
            </div>

            {sourceType === 'image' && (
              <div>
                <label className="block text-sm font-medium text-text-secondary mb-1">Docker Image</label>
                <input type="text" value={config.sourceURL} onChange={(e) => setConfig({ ...config, sourceURL: e.target.value })}
                  className="w-full px-3 py-2 rounded-lg border border-border bg-surface text-text-primary focus:ring-2 focus:ring-monster-green/50"
                  placeholder="nginx:latest" />
              </div>
            )}

            {sourceType === 'git' && (
              <>
                <div>
                  <label className="block text-sm font-medium text-text-secondary mb-1">Repository URL</label>
                  <input type="text" value={config.sourceURL} onChange={(e) => setConfig({ ...config, sourceURL: e.target.value })}
                    className="w-full px-3 py-2 rounded-lg border border-border bg-surface text-text-primary focus:ring-2 focus:ring-monster-green/50"
                    placeholder="https://github.com/user/repo.git" />
                </div>
                <div>
                  <label className="block text-sm font-medium text-text-secondary mb-1">Branch</label>
                  <input type="text" value={config.branch} onChange={(e) => setConfig({ ...config, branch: e.target.value })}
                    className="w-full px-3 py-2 rounded-lg border border-border bg-surface text-text-primary focus:ring-2 focus:ring-monster-green/50"
                    placeholder="main" />
                </div>
              </>
            )}
          </div>
          <div className="flex justify-between">
            <button onClick={() => setStep(0)} className="flex items-center gap-2 px-4 py-2 border border-border text-text-secondary text-sm rounded-lg hover:bg-surface-secondary">
              <ArrowLeft size={16} /> Back
            </button>
            <button onClick={() => config.name && setStep(2)} disabled={!config.name}
              className="flex items-center gap-2 px-4 py-2 bg-monster-green hover:bg-monster-green-dark text-white text-sm rounded-lg disabled:opacity-50 transition-colors">
              Next <ArrowRight size={16} />
            </button>
          </div>
        </div>
      )}

      {/* Step 3: Deploy */}
      {step === 2 && (
        <div className="space-y-4">
          <h2 className="font-medium text-text-primary">Review and deploy</h2>
          <div className="bg-surface border border-border rounded-xl p-6 space-y-3 text-sm">
            <div className="flex justify-between py-1"><span className="text-text-secondary">Name</span><span className="text-text-primary font-medium">{config.name}</span></div>
            <div className="flex justify-between py-1"><span className="text-text-secondary">Source</span><span className="text-text-primary">{sourceType}</span></div>
            {config.sourceURL && <div className="flex justify-between py-1"><span className="text-text-secondary">URL</span><span className="text-text-primary truncate max-w-64">{config.sourceURL}</span></div>}
            {sourceType === 'git' && <div className="flex justify-between py-1"><span className="text-text-secondary">Branch</span><span className="text-text-primary">{config.branch}</span></div>}
          </div>

          {error && <div className="bg-red-50 dark:bg-red-900/20 text-red-600 dark:text-red-400 text-sm px-3 py-2 rounded-lg">{error}</div>}

          <div className="flex justify-between">
            <button onClick={() => setStep(1)} className="flex items-center gap-2 px-4 py-2 border border-border text-text-secondary text-sm rounded-lg hover:bg-surface-secondary">
              <ArrowLeft size={16} /> Back
            </button>
            <button onClick={handleDeploy} disabled={deploying}
              className="flex items-center gap-2 px-5 py-2.5 bg-monster-green hover:bg-monster-green-dark text-white font-medium rounded-lg disabled:opacity-50 transition-colors">
              <Rocket size={16} /> {deploying ? 'Deploying...' : 'Deploy Application'}
            </button>
          </div>
        </div>
      )}
    </div>
  );
}
