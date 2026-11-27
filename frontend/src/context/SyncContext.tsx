import { createContext, useContext, useState, useCallback, useEffect, useRef, ReactNode } from 'react';
import { useSocket } from './SocketContext';
import { useQueryClient } from '@tanstack/react-query';
import { api, type SyncStatus } from '../services/api';

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
  /** The sync request was accepted: follow the sync through the status endpoint as well. */
  syncAccepted: () => void;
}

/** How often the backend is asked for the sync state while a sync is shown. */
const STATUS_POLL_MS = 2000;

const SyncContext = createContext<SyncContextType | undefined>(undefined);

export function SyncProvider({ children }: { children: ReactNode }) {
  const [sync, setSync] = useState<SyncProgress>({
    isSyncing: false,
    syncedCount: 0,
    totalToSync: 0,
    progress: 0,
    currentRepo: null,
  });

  // Set once the backend accepted the request. The WebSocket normally reports the progress; the
  // status endpoint covers the cases where it cannot (socket down, or blocked by a proxy).
  const [following, setFollowing] = useState(false);

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
    setFollowing(false);
    setSync(prev => ({ ...prev, isSyncing: false }));
  }, []);

  const syncAccepted = useCallback(() => setFollowing(true), []);

  // The sync is over (reported by the WebSocket or by the status endpoint): reload the data and let
  // the finished bar show for a moment.
  const completeSync = useCallback(() => {
    setFollowing(false);
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
  }, [cancelReset, queryClient]);

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
        case 'sync:complete':
          completeSync();
          break;
        case 'sync:error': {
          setFollowing(false);
          setSync(prev => ({
            ...prev,
            isSyncing: false,
          }));
          break;
        }
      }
    });
  }, [subscribe, cancelReset, completeSync]);

  // While a sync is shown, also ask the backend for its state.
  useEffect(() => {
    if (!following) return;
    let cancelled = false;
    const poll = async () => {
      let status: SyncStatus;
      try {
        status = await api.syncStatus();
      } catch {
        return; // transient: the next tick asks again
      }
      if (cancelled) return;
      if (status.running) {
        setSync(prev => ({
          ...prev,
          isSyncing: true,
          syncedCount: status.synced,
          totalToSync: status.total,
          // The WebSocket may already have reported a later step; the bar never moves backwards.
          progress: Math.max(
            prev.progress,
            status.total > 0 ? Math.round((status.synced / status.total) * 100) : 0,
          ),
          currentRepo: status.current || prev.currentRepo,
        }));
      } else if (status.error) {
        setFollowing(false);
        setSync(prev => ({ ...prev, isSyncing: false }));
      } else {
        completeSync();
      }
    };
    const id = setInterval(() => void poll(), STATUS_POLL_MS);
    return () => {
      cancelled = true;
      clearInterval(id);
    };
  }, [following, completeSync]);

  return (
    <SyncContext.Provider value={{ sync, startSync, failSync, syncAccepted }}>
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

