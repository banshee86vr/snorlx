import { GitBranch, Loader2 } from 'lucide-react';
import { useState } from 'react';

function GitHubMark({ className }: { className?: string }) {
  return (
    <svg viewBox="0 0 24 24" className={className} fill="currentColor" aria-hidden>
      <path d="M12 0c-6.626 0-12 5.373-12 12 0 5.302 3.438 9.8 8.207 11.387.599.111.793-.261.793-.577v-2.234c-3.338.726-4.033-1.416-4.033-1.416-.546-1.387-1.333-1.756-1.333-1.756-1.089-.745.083-.729.083-.729 1.205.084 1.839 1.237 1.839 1.237 1.07 1.834 2.807 1.304 3.492.997.107-.775.418-1.305.762-1.604-2.665-.305-5.467-1.334-5.467-5.931 0-1.311.469-2.381 1.236-3.221-.124-.303-.535-1.524.117-3.176 0 0 1.008-.322 3.301 1.23.957-.266 1.983-.399 3.003-.404 1.02.005 2.047.138 3.006.404 2.291-1.552 3.297-1.23 3.297-1.23.653 1.653.242 2.874.118 3.176.77.84 1.235 1.911 1.235 3.221 0 4.609-2.807 5.624-5.479 5.921.43.372.823 1.102.823 2.222v3.293c0 .319.192.694.801.576 4.765-1.589 8.199-6.086 8.199-11.386 0-6.627-5.373-12-12-12z" />
    </svg>
  );
}

export function Login() {
  const [isLoading, setIsLoading] = useState(false);
  
  const handleLogin = () => {
    setIsLoading(true);
    // Redirect to backend OAuth endpoint
    window.location.href = `${import.meta.env.VITE_API_URL || ''}/api/auth/login`;
  };
  
  return (
    <div className="min-h-screen bg-slate-950 flex items-center justify-center p-4 relative overflow-hidden">
      {/* Abstract background pattern with neon glow */}
      <div className="absolute inset-0 overflow-hidden">
        {/* Animated gradient orbs with random floating movement */}
        <div 
          className="absolute top-0 -left-1/4 w-[900px] h-[900px] bg-linear-to-br from-primary-500/60 to-primary-600/50 rounded-full blur-[140px]"
          style={{ animation: 'float1 10s ease-in-out infinite' }}
        ></div>
        <div 
          className="absolute bottom-0 -right-1/4 w-[1000px] h-[1000px] bg-linear-to-tl from-secondary-600/60 to-secondary-700/50 rounded-full blur-[140px]"
          style={{ animation: 'float2 12s ease-in-out infinite' }}
        ></div>
        <div 
          className="absolute top-1/3 left-1/3 w-[800px] h-[800px] bg-linear-to-r from-primary-500/50 via-cyan-500/50 to-secondary-600/50 rounded-full blur-[140px]"
          style={{ animation: 'float3 14s ease-in-out infinite' }}
        ></div>
        
        {/* Additional smaller orbs for depth */}
        <div 
          className="absolute top-1/4 right-1/4 w-[500px] h-[500px] bg-primary-400/35 rounded-full blur-[120px]"
          style={{ animation: 'float2 12s ease-in-out infinite 3s' }}
        ></div>
        <div 
          className="absolute bottom-1/4 left-1/3 w-[550px] h-[550px] bg-secondary-500/35 rounded-full blur-[120px]"
          style={{ animation: 'float1 10s ease-in-out infinite 5s' }}
        ></div>
        
        {/* Grid pattern overlay */}
        <div className="absolute inset-0 bg-[linear-gradient(rgba(52,203,111,0.03)_1px,transparent_1px),linear-gradient(90deg,rgba(52,203,111,0.03)_1px,transparent_1px)] bg-size-[64px_64px]"></div>
        
        {/* Radial gradient overlay for vignette effect */}
        <div className="absolute inset-0 bg-[radial-gradient(circle_at_center,transparent_0%,rgba(2,6,23,0.5)_50%,rgba(2,6,23,0.8)_100%)]"></div>
      </div>

      <div className="w-full max-w-md relative z-10">
        {/* Logo and Title */}
        <div className="text-center mb-8">
          <div className="inline-flex items-center justify-center w-16 h-16 rounded-lg bg-linear-to-br from-primary-500 to-secondary-600 text-white mb-4 shadow-lg shadow-primary-500/50 animate-glow">
            <GitBranch className="w-10 h-10" />
          </div>
          <h1 className="text-3xl font-bold text-white mb-2">Snorlx</h1>
          <p className="text-gray-400">CI/CD Dashboard for GitHub Actions</p>
        </div>

        {/* Login Card */}
        <div className="bg-slate-800/50 backdrop-blur-xl rounded-xl border border-slate-700/50 shadow-2xl shadow-primary-500/10 p-8">
          <div className="mb-6">
            <h2 className="text-xl font-semibold text-white mb-2">Welcome back</h2>
            <p className="text-gray-400 text-sm">
              Sign in with your GitHub account to access your CI/CD pipelines and metrics.
            </p>
          </div>
          
          <button
            onClick={handleLogin}
            disabled={isLoading}
            className="w-full flex items-center justify-center gap-3 px-6 py-3 bg-slate-700 hover:bg-slate-600 text-white rounded-lg font-medium transition-colors border border-slate-600 hover:border-slate-500 disabled:opacity-50 disabled:cursor-not-allowed disabled:hover:bg-slate-700"
          >
            {isLoading ? (
              <>
                <Loader2 className="w-5 h-5 animate-spin" />
                Signing in...
              </>
            ) : (
              <>
                <GitHubMark className="w-5 h-5" />
                Sign in with GitHub
              </>
            )}
          </button>
          
          <div className="mt-6 text-center text-sm text-gray-400">
            <p>Requires a GitHub App installation</p>
            <p className="text-xs mt-2 text-gray-500">
              Your organization admin must install the Snorlx GitHub App
            </p>
          </div>
        </div>
        
        {/* Footer */}
        <div className="mt-8 text-center text-xs text-gray-500">
          <p>By signing in, you agree to our terms and conditions</p>
        </div>
      </div>
    </div>
  );
}
