import { createContext, useContext, useState, useCallback, useEffect, ReactNode } from 'react';
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

  const { lastMessage } = useSocket();
  const queryClient = useQueryClient();

  const startSync = useCallback(() => {
    setSync({
      isSyncing: true,
      syncedCount: 0,
      totalToSync: 0,
      progress: 0,
      currentRepo: null,
    });
  }, []);

  // Handle WebSocket messages for sync events (defer state updates to satisfy react-hooks/set-state-in-effect)
  useEffect(() => {
    if (!lastMessage) return;

    const { type, data } = lastMessage as { type: string; data: Record<string, unknown> };

    const apply = () => {
      switch (type) {
        case 'sync:start': {
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
          queryClient.invalidateQueries({ queryKey: ['workflows'] });
          queryClient.invalidateQueries({ queryKey: ['runs'] });
          queryClient.invalidateQueries({ queryKey: ['dashboard'] });
          queryClient.invalidateQueries({ queryKey: ['metrics'] });
          // Reset after 2 seconds
          setTimeout(() => {
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
    };
    queueMicrotask(apply);
  }, [lastMessage, queryClient]);

  return (
    <SyncContext.Provider value={{ sync, startSync }}>
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

