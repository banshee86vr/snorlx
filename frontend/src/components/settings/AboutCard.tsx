import { ExternalLink, Info } from 'lucide-react';

const releaseUrl = `https://github.com/banshee86vr/snorlx/releases/tag/v${__APP_VERSION__}`;

export function AboutCard() {
  return (
    <div className="card">
      <div className="border-b border-gray-200 px-4 py-4 sm:px-6 dark:border-gray-700">
        <div className="flex items-center gap-2">
          <Info className="h-5 w-5 text-gray-500" />
          <h2 className="text-lg font-semibold text-gray-900 dark:text-gray-100">About</h2>
        </div>
      </div>
      <div className="p-4 sm:p-6">
        <div className="flex flex-col gap-2 rounded-lg bg-gray-50 p-4 sm:flex-row sm:items-center sm:justify-between dark:bg-gray-800">
          <div className="min-w-0">
            <p className="font-medium text-gray-900 dark:text-gray-100">Version</p>
            <p className="text-sm text-gray-500 dark:text-gray-400">Frontend build</p>
          </div>
          <span className="text-sm text-gray-600 dark:text-gray-300">{__APP_VERSION__}</span>
        </div>
        <a
          href={releaseUrl}
          target="_blank"
          rel="noopener noreferrer"
          className="mt-4 inline-flex items-center gap-1 text-sm text-gray-600 hover:text-gray-900 dark:text-gray-300 dark:hover:text-gray-100"
        >
          GitHub release
          <ExternalLink className="h-4 w-4" />
        </a>
      </div>
    </div>
  );
}
