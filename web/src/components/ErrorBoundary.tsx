import React from 'react';
import { ReportUIError } from '../../wailsjs/go/main/App';

interface ErrorBoundaryProps {
  children: React.ReactNode;
}

interface ErrorBoundaryState {
  error: Error | null;
  copied: boolean;
}

/**
 * Last line of defense against the Wails "black screen" failure mode.
 *
 * Every prior incident in this repo (missing `window.runtime` at mount,
 * hook-count drift in UpdateBanner, a throwing EventEmitter) ended the
 * same way: an uncaught render error unmounted the React tree, the
 * transparent webview let the zinc-950 BackgroundColour show through,
 * and users saw an unresponsive black window with no signal.
 *
 * This boundary converts that into a visible, actionable fallback with
 * the error message + a one-click reload. It is intentionally a class
 * component — the only place in the codebase where one is warranted,
 * because `componentDidCatch` has no function-component equivalent.
 */
class ErrorBoundary extends React.Component<ErrorBoundaryProps, ErrorBoundaryState> {
  constructor(props: ErrorBoundaryProps) {
    super(props);
    this.state = { error: null, copied: false };
  }

  static getDerivedStateFromError(error: Error): ErrorBoundaryState {
    return { error, copied: false };
  }

  componentDidCatch(error: Error, info: React.ErrorInfo): void {
    console.error('ClashGO UI crashed:', error, info.componentStack);
    try {
      void ReportUIError(error.message || String(error), info.componentStack || '').catch(() => {});
    } catch {
      // The Wails bridge may not exist when running the frontend directly.
    }
  }

  handleReload = (): void => {
    // Full reload re-runs the theme bootstrap + re-mounts React. If the
    // crash was transient (bridge not injected yet, one bad event
    // payload) this recovers cleanly; if persistent, the boundary
    // catches again rather than black-screening.
    window.location.reload();
  };

  handleCopy = async (): Promise<void> => {
    const message = this.state.error?.message || String(this.state.error || '');
    try {
      await navigator.clipboard.writeText(message);
    } catch {
      const ta = document.createElement('textarea');
      ta.value = message;
      ta.style.position = 'fixed';
      ta.style.opacity = '0';
      document.body.appendChild(ta);
      ta.select();
      document.execCommand('copy');
      document.body.removeChild(ta);
    }
    this.setState({ copied: true });
  };

  render() {
    if (this.state.error) {
      return (
        <div className="min-h-screen w-full flex items-center justify-center bg-zinc-50 dark:bg-zinc-950 text-zinc-950 dark:text-zinc-50 p-8">
          <div className="max-w-md w-full bg-white dark:bg-zinc-900 rounded-[2rem] border border-zinc-100/50 dark:border-zinc-800/50 shadow-premium-lg p-8 text-center space-y-6">
            <div className="w-16 h-16 mx-auto rounded-2xl bg-rose-500/10 border border-rose-500/30 flex items-center justify-center">
              <span className="material-symbols-outlined text-rose-500 text-3xl">error</span>
            </div>
            <div className="space-y-2">
              <h1 className="font-headline text-xl font-bold tracking-tight">ClashGO a rencontré un problème</h1>
              <p className="text-sm text-zinc-400 dark:text-zinc-500 font-medium">
                L’interface a rencontré une erreur inattendue. Un redémarrage de l’interface suffit généralement à la corriger.
              </p>
            </div>
            <pre className="max-h-32 overflow-y-auto text-left text-[11px] font-mono text-rose-500/80 bg-rose-500/5 border border-rose-500/20 rounded-xl p-3 break-words whitespace-pre-wrap">
              {this.state.error.message || String(this.state.error)}
            </pre>
            <div className="grid grid-cols-1 sm:grid-cols-2 gap-2">
              <button
                onClick={() => void this.handleCopy()}
                className="h-12 w-full rounded-2xl border border-zinc-200 dark:border-zinc-700 text-zinc-700 dark:text-zinc-200 font-black text-[10px] uppercase tracking-[0.2em] transition-all hover:bg-zinc-50 dark:hover:bg-zinc-800 active:scale-[0.98]"
              >
                {this.state.copied ? 'Erreur copiée' : 'Copier l’erreur'}
              </button>
              <button
                onClick={this.handleReload}
                className="h-12 w-full rounded-2xl bg-zinc-950 dark:bg-white text-white dark:text-zinc-950 font-black text-[10px] uppercase tracking-[0.2em] transition-all hover:shadow-premium-lg active:scale-[0.98]"
              >
                Relancer l’interface
              </button>
            </div>
          </div>
        </div>
      );
    }
    return this.props.children;
  }
}

export default ErrorBoundary;
