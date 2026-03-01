import React, { createContext, useContext, useReducer, useCallback, ReactNode, useEffect } from 'react';
import { AppState as ReactNativeAppState, AppStateStatus } from 'react-native';
import AsyncStorage from '@react-native-async-storage/async-storage';
import LibrCore, { RetMsgCert } from '@/modules/LibrCore';

// ── Types ─────────────────────────────────────────────────────────────────────

export type ConnectionStatus = 'idle' | 'initializing' | 'connected' | 'error';

interface AppState {
  /** Ed25519 public key (base64) of this device. */
  publicKey: string;
  /** libp2p Peer ID for this device. */
  peerId: string;
  /** Current network connection/init status. */
  connectionStatus: ConnectionStatus;
  /** Last error message, if any. */
  lastError: string | null;
  /** All fetched messages (most-recent first). */
  messages: RetMsgCert[];
  /** Whether a message fetch is in progress. */
  isFetching: boolean;
  /** Set of message signatures reported by the current user. */
  reportedSigns: Set<string>;
}

type Action =
  | { type: 'SET_PUBLIC_KEY'; payload: string }
  | { type: 'SET_PEER_ID'; payload: string }
  | { type: 'SET_CONNECTION_STATUS'; payload: ConnectionStatus }
  | { type: 'SET_ERROR'; payload: string | null }
  | { type: 'SET_MESSAGES'; payload: RetMsgCert[] }
  | { type: 'SET_FETCHING'; payload: boolean }
  | { type: 'REMOVE_MESSAGE'; payload: string }
  | { type: 'ADD_REPORTED_SIGN'; payload: string }
  | { type: 'SET_REPORTED_SIGNS'; payload: Set<string> };

// ── Reducer ───────────────────────────────────────────────────────────────────

const initialState: AppState = {
  publicKey: '',
  peerId: '',
  connectionStatus: 'idle',
  lastError: null,
  messages: [],
  isFetching: false,
  reportedSigns: new Set(),
};

function reducer(state: AppState, action: Action): AppState {
  switch (action.type) {
    case 'SET_PUBLIC_KEY': return { ...state, publicKey: action.payload };
    case 'SET_PEER_ID': return { ...state, peerId: action.payload };
    case 'SET_CONNECTION_STATUS': return { ...state, connectionStatus: action.payload };
    case 'SET_ERROR': return { ...state, lastError: action.payload };
    case 'SET_MESSAGES': return { ...state, messages: action.payload };
    case 'SET_FETCHING': return { ...state, isFetching: action.payload };
    case 'REMOVE_MESSAGE': return { ...state, messages: state.messages.filter(m => m.sign !== action.payload) };
    case 'ADD_REPORTED_SIGN': {
      const newSet = new Set(state.reportedSigns);
      newSet.add(action.payload);
      return { ...state, reportedSigns: newSet };
    }
    case 'SET_REPORTED_SIGNS': return { ...state, reportedSigns: action.payload };
    default: return state;
  }
}

// ── Context ───────────────────────────────────────────────────────────────────

interface StoreContextValue {
  state: AppState;
  setPublicKey: (key: string) => void;
  setPeerId: (id: string) => void;
  setConnectionStatus: (status: ConnectionStatus) => void;
  setError: (err: string | null) => void;
  setMessages: (msgs: RetMsgCert[]) => void;
  setFetching: (v: boolean) => void;
  removeMessage: (sign: string) => void;
  addReportedSign: (sign: string) => void;
}

const StoreContext = createContext<StoreContextValue | undefined>(undefined);
const REPORTED_SIGNS_KEY = '@libr_reported_signs';

export function AppStoreProvider({ children }: { children: ReactNode }) {
  const [state, dispatch] = useReducer(reducer, initialState);

  // Load reported signs on mount
  useEffect(() => {
    AsyncStorage.getItem(REPORTED_SIGNS_KEY).then((data: string | null) => {
      if (data) {
        try {
          const arr = JSON.parse(data);
          if (Array.isArray(arr)) {
            dispatch({ type: 'SET_REPORTED_SIGNS', payload: new Set(arr) });
          }
        } catch { /* ignore parse error */ }
      }
    });
  }, []);

  // Manage Go Cron Job Lifecycle
  useEffect(() => {
    const subscription = ReactNativeAppState.addEventListener('change', nextAppState => {
      if (nextAppState === 'active') {
        LibrCore.startCron().catch(console.warn);
      } else if (nextAppState === 'background') {
        LibrCore.stopCron().catch(console.warn);
      }
    });

    // Start immediately if already active
    if (ReactNativeAppState.currentState === 'active') {
      LibrCore.startCron().catch(console.warn);
    }

    return () => {
      subscription.remove();
      LibrCore.stopCron().catch(console.warn);
    };
  }, []);

  const setPublicKey = useCallback((key: string) => dispatch({ type: 'SET_PUBLIC_KEY', payload: key }), []);
  const setPeerId = useCallback((id: string) => dispatch({ type: 'SET_PEER_ID', payload: id }), []);
  const setConnectionStatus = useCallback((s: ConnectionStatus) => dispatch({ type: 'SET_CONNECTION_STATUS', payload: s }), []);
  const setError = useCallback((e: string | null) => dispatch({ type: 'SET_ERROR', payload: e }), []);
  const setMessages = useCallback((m: RetMsgCert[]) => dispatch({ type: 'SET_MESSAGES', payload: m }), []);
  const setFetching = useCallback((v: boolean) => dispatch({ type: 'SET_FETCHING', payload: v }), []);
  const removeMessage = useCallback((sign: string) => dispatch({ type: 'REMOVE_MESSAGE', payload: sign }), []);
  const addReportedSign = useCallback((sign: string) => {
    dispatch({ type: 'ADD_REPORTED_SIGN', payload: sign });
    // Persist to AsyncStorage
    AsyncStorage.getItem(REPORTED_SIGNS_KEY).then((data: string | null) => {
      const arr = data ? JSON.parse(data) : [];
      if (!arr.includes(sign)) {
        arr.push(sign);
        AsyncStorage.setItem(REPORTED_SIGNS_KEY, JSON.stringify(arr));
      }
    }).catch(() => { });
  }, []);

  return (
    <StoreContext.Provider value={{ state, setPublicKey, setPeerId, setConnectionStatus, setError, setMessages, setFetching, removeMessage, addReportedSign }}>
      {children}
    </StoreContext.Provider>
  );
}

export function useAppStore() {
  const ctx = useContext(StoreContext);
  if (!ctx) throw new Error('useAppStore must be used inside <AppStoreProvider>');
  return ctx;
}
