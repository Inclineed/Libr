/**
 * Minimal reactive store for the Libr mobile app.
 * Uses React Context + useReducer so no external dependencies are needed.
 */
import React, { createContext, useContext, useReducer, useCallback, ReactNode } from 'react';
import { RetMsgCert } from '@/modules/LibrCore';

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
}

type Action =
  | { type: 'SET_PUBLIC_KEY'; payload: string }
  | { type: 'SET_PEER_ID'; payload: string }
  | { type: 'SET_CONNECTION_STATUS'; payload: ConnectionStatus }
  | { type: 'SET_ERROR'; payload: string | null }
  | { type: 'SET_MESSAGES'; payload: RetMsgCert[] }
  | { type: 'SET_FETCHING'; payload: boolean }
  | { type: 'REMOVE_MESSAGE'; payload: string }; // sign of the message to remove

// ── Reducer ───────────────────────────────────────────────────────────────────

const initialState: AppState = {
  publicKey: '',
  peerId: '',
  connectionStatus: 'idle',
  lastError: null,
  messages: [],
  isFetching: false,
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
}

const StoreContext = createContext<StoreContextValue | undefined>(undefined);

export function AppStoreProvider({ children }: { children: ReactNode }) {
  const [state, dispatch] = useReducer(reducer, initialState);

  const setPublicKey = useCallback((key: string) => dispatch({ type: 'SET_PUBLIC_KEY', payload: key }), []);
  const setPeerId = useCallback((id: string) => dispatch({ type: 'SET_PEER_ID', payload: id }), []);
  const setConnectionStatus = useCallback((s: ConnectionStatus) => dispatch({ type: 'SET_CONNECTION_STATUS', payload: s }), []);
  const setError = useCallback((e: string | null) => dispatch({ type: 'SET_ERROR', payload: e }), []);
  const setMessages = useCallback((m: RetMsgCert[]) => dispatch({ type: 'SET_MESSAGES', payload: m }), []);
  const setFetching = useCallback((v: boolean) => dispatch({ type: 'SET_FETCHING', payload: v }), []);
  const removeMessage = useCallback((sign: string) => dispatch({ type: 'REMOVE_MESSAGE', payload: sign }), []);

  return (
    <StoreContext.Provider value={{ state, setPublicKey, setPeerId, setConnectionStatus, setError, setMessages, setFetching, removeMessage }}>
      {children}
    </StoreContext.Provider>
  );
}

export function useAppStore() {
  const ctx = useContext(StoreContext);
  if (!ctx) throw new Error('useAppStore must be used inside <AppStoreProvider>');
  return ctx;
}
