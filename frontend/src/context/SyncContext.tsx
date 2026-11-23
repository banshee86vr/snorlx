import { createContext, useContext, useState, useCallback, useEffect, useRef, ReactNode } from 'react';
import { useSocket } from './SocketContext';
import { useQueryClient } from '@tanstack/react-query';

interface SyncProgress {
  isSyncing: boolean;
  syncedCount: number;
  totalToSync: number;
  progress: number;
  currentRepo: string | null;
}

interface SyncContextType {
  sync: SyncProgress;
  startSync: () => void;
  /** Ends the progress display after the sync request was rejected. */
  failSync: () => void;
}

const SyncContext = createContext<SyncContextType | undefined>(undefined);

export function SyncProvider({ children }: { children: ReactNode }) {
  const [sync, setSync] = useState<SyncProgress>({
    isSyncing: false,
    syncedCount: 0,
    totalToSync: 0,
    progress: 0,
    currentRepo: null,
  });

  const { subscribe } = useSocket();
  const queryClient = useQueryClient();
  // The pending "reset after complete" timer; a new sync must cancel it or it would wipe its progress.
  const resetTimer = useRef<ReturnType<typeof setTimeout> | null>(null);

  const cancelReset = useCallback(() => {
    if (resetTimer.current) {
      clearTimeout(resetTimer.current);
      resetTimer.current = null;
    }
  }, []);

  useEffect(() => cancelReset, [cancelReset]);

  const failSync = useCallback(() => {
    setSync(prev => ({ ...prev, isSyncing: false }));
  }, []);

  const startSync = useCallback(() => {
    cancelReset();
    setSync({
      isSyncing: true,
      syncedCount: 0,
      totalToSync: 0,
      progress: 0,
      currentRepo: null,
    });
  }, [cancelReset]);

  // Handle WebSocket messages for sync events. Every message is delivered, in order: the backend
  // sends start and the first progress back to back, and a "latest message" state would drop one.
  useEffect(() => {
    return subscribe(message => {
      const { type } = message;
      const data = (message.data ?? {}) as Record<string, unknown>;

      switch (type) {
        case 'sync:start': {
          cancelReset();
          setSync({
            isSyncing: true,
            syncedCount: 0,
            totalToSync: (data.total as number) || 0,
            progress: 0,
            currentRepo: null,
          });
          break;
        }
        case 'sync:progress': {
          const synced = (data.synced as number) || 0;
          const total = (data.total as number) || 0;
          const progress = total > 0 ? Math.round((synced / total) * 100) : 0;
          setSync(prev => ({
            ...prev,
            isSyncing: true,
            syncedCount: synced,
            totalToSync: total,
            progress,
            currentRepo: (data.current as string) || null,
          }));
          break;
        }
        case 'sync:complete': {
          setSync(prev => ({
            ...prev,
            isSyncing: false,
            progress: 100,
          }));
          // Refresh all data
          queryClient.invalidateQueries({ queryKey: ['repositories'] });
          queryClient.invalidateQueries({ queryKey: ['repositories', 'scores'] });
          queryClient.invalidateQueries({ queryKey: ['workflows'] });
          queryClient.invalidateQueries({ queryKey: ['runs'] });
          queryClient.invalidateQueries({ queryKey: ['dashboard'] });
          queryClient.invalidateQueries({ queryKey: ['pipelines'] });
          // Reset after 2 seconds
          cancelReset();
          resetTimer.current = setTimeout(() => {
            resetTimer.current = null;
            setSync({
              isSyncing: false,
              syncedCount: 0,
              totalToSync: 0,
              progress: 0,
              currentRepo: null,
            });
          }, 2000);
          break;
        }
        case 'sync:error': {
          setSync(prev => ({
            ...prev,
            isSyncing: false,
          }));
          break;
        }
      }
    });
  }, [subscribe, cancelReset, queryClient]);

  return (
    <SyncContext.Provider value={{ sync, startSync, failSync }}>
      {children}
    </SyncContext.Provider>
  );
}

export function useSync() {
  const context = useContext(SyncContext);
  if (!context) {
    throw new Error('useSync must be used within SyncProvider');
  }
  return context;
}

