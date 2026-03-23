import { useState } from 'react';
import { useNavigate } from 'react-router';
import { Rocket, Server, Globe, GitBranch, Check } from 'lucide-react';

const steps = [
  { icon: Rocket, title: 'Welcome', desc: 'Let\'s set up your platform' },
  { icon: Server, title: 'Server', desc: 'Configure your server' },
  { icon: Globe, title: 'Domain', desc: 'Set your platform domain' },
  { icon: GitBranch, title: 'Git', desc: 'Connect a Git provider' },
  { icon: Check, title: 'Done', desc: 'You\'re all set!' },
];

export function Onboarding() {
  const navigate = useNavigate();
  const [step, setStep] = useState(0);
  const [config, setConfig] = useState({
    domain: '',
    gitProvider: '',
  });

  const next = () => {
    if (step < steps.length - 1) setStep(step + 1);
    else {
      localStorage.setItem('onboarding_complete', 'true');
      navigate('/');
    }
  };

  return (
    <div className="min-h-screen bg-surface flex items-center justify-center p-4">
      <div className="w-full max-w-lg space-y-8">
        {/* Progress dots */}
        <div className="flex justify-center gap-2">
          {steps.map((_, i) => (
            <div key={i} className={`w-2.5 h-2.5 rounded-full transition-colors ${
              i <= step ? 'bg-monster-green' : 'bg-border'
            }`} />
          ))}
        </div>

        {/* Step content */}
        <div className="bg-surface border border-border rounded-2xl p-8 text-center">
          {step === 0 && (
            <div className="space-y-4">
              <div className="w-16 h-16 rounded-2xl bg-monster-green mx-auto flex items-center justify-center text-white">
                <Rocket size={32} />
              </div>
              <h1 className="text-2xl font-semibold text-text-primary">Welcome to DeployMonster</h1>
              <p className="text-text-secondary">Your self-hosted PaaS is ready. Let's configure a few things to get you started.</p>
            </div>
          )}

          {step === 1 && (
            <div className="space-y-4 text-left">
              <h2 className="text-xl font-semibold text-text-primary text-center">Server Configuration</h2>
              <p className="text-sm text-text-secondary text-center">Your server is already running. Here's what we detected:</p>
              <div className="bg-surface-secondary rounded-xl p-4 space-y-2 text-sm">
                <div className="flex justify-between"><span className="text-text-secondary">Hostname</span><span className="text-text-primary font-mono">localhost</span></div>
                <div className="flex justify-between"><span className="text-text-secondary">Docker</span><span className="text-status-running">Connected</span></div>
                <div className="flex justify-between"><span className="text-text-secondary">SSL</span><span className="text-text-primary">Auto (Let's Encrypt)</span></div>
                <div className="flex justify-between"><span className="text-text-secondary">Ports</span><span className="text-text-primary">80, 443, 8443</span></div>
              </div>
            </div>
          )}

          {step === 2 && (
            <div className="space-y-4">
              <h2 className="text-xl font-semibold text-text-primary">Platform Domain</h2>
              <p className="text-sm text-text-secondary">Set your platform's domain for accessing the dashboard and auto-subdomains.</p>
              <input type="text" value={config.domain} onChange={(e) => setConfig({ ...config, domain: e.target.value })}
                className="w-full px-4 py-3 rounded-xl border border-border bg-surface text-text-primary text-center focus:ring-2 focus:ring-monster-green/50"
                placeholder="deploy.example.com" />
              <p className="text-xs text-text-muted">Apps will get subdomains like app-name.deploy.example.com</p>
            </div>
          )}

          {step === 3 && (
            <div className="space-y-4">
              <h2 className="text-xl font-semibold text-text-primary">Connect Git Provider</h2>
              <p className="text-sm text-text-secondary">Connect a Git provider to enable auto-deploy from push.</p>
              <div className="grid grid-cols-3 gap-3">
                {['GitHub', 'GitLab', 'Gitea'].map((p) => (
                  <button key={p} onClick={() => setConfig({ ...config, gitProvider: p.toLowerCase() })}
                    className={`p-4 rounded-xl border text-sm font-medium transition-colors ${
                      config.gitProvider === p.toLowerCase()
                        ? 'border-monster-green bg-monster-green/5 text-monster-green'
                        : 'border-border text-text-secondary hover:border-monster-green/30'
                    }`}>
                    {p}
                  </button>
                ))}
              </div>
              <button className="text-sm text-text-muted hover:text-text-secondary">Skip for now</button>
            </div>
          )}

          {step === 4 && (
            <div className="space-y-4">
              <div className="w-16 h-16 rounded-2xl bg-monster-green mx-auto flex items-center justify-center text-white">
                <Check size={32} />
              </div>
              <h2 className="text-xl font-semibold text-text-primary">You're All Set!</h2>
              <p className="text-text-secondary">Your DeployMonster platform is configured and ready to deploy.</p>
              <div className="bg-surface-secondary rounded-xl p-4 text-sm text-left space-y-2">
                <p className="font-medium text-text-primary">Quick actions:</p>
                <ul className="space-y-1 text-text-secondary">
                  <li>Deploy an app from the Marketplace</li>
                  <li>Connect your Git repository</li>
                  <li>Deploy a Docker image</li>
                  <li>Invite your team members</li>
                </ul>
              </div>
            </div>
          )}

          {/* Next button */}
          <button onClick={next}
            className="mt-6 w-full py-3 bg-monster-green hover:bg-monster-green-dark text-white font-medium rounded-xl transition-colors">
            {step === steps.length - 1 ? 'Go to Dashboard' : 'Continue'}
          </button>
        </div>
      </div>
    </div>
  );
}
