import React from 'react';
import { BrowserOpenURL } from '../../../wailsjs/runtime';
import { PencilLine, Globe, Database, Copyright, ChevronDown, ChevronRight, ImageIcon, FileText, Flag, CheckCircle2, XCircle, X } from 'lucide-react';
import { logger } from '../../logger/logger';
import { EventsOn } from "../../../wailsjs/runtime";
import { toast } from "sonner";
import {
  GetOnlineMods,
  GenerateAlias,
  GenerateAvatar,
  GetPendingModerationStats
} from "../../../wailsjs/go/main/App";
import { useAppStore, PendingModeration } from '../../store/useAppStore';

type ModDisplay = {
  key: string;
  alias: string;
  avatarSvg: string;
};

const ComingSoonDialog: React.FC<{ open: boolean; onClose: () => void }> = ({ open, onClose }) => {
  if (!open) return null;
  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/40">
      <div className="bg-card border border-border/50 rounded-2xl shadow-xl text-foreground p-6 w-[90%] max-w-md flex flex-col ">
        <span className="text-lg font-semibold mb-4 text-libr-secondary">Feature Coming Soon</span>
        <p className="text-muted-foreground mb-6 text-left">
          This feature is not available yet. Stay tuned for updates!
        </p>
        <div className="flex justify-end space-x-2">
          <button
            onClick={onClose}
            className="libr-button bg-muted hover:bg-muted/70 text-foreground px-6 py-2"
          >
            Close
          </button>
        </div>
      </div>
    </div>
  );
};


export const Menubar: React.FC = () => {
  type ResolvedItem = PendingModeration & { finalStatus: 'approved' | 'rejected' };

  const [mods, setMods] = React.useState<ModDisplay[]>([]);
  const [dialogOpen, setDialogOpen] = React.useState(false);
  const [queueOpen, setQueueOpen] = React.useState(true);
  const [expandedItem, setExpandedItem] = React.useState<string | null>(null);
  const [resolvedItems, setResolvedItems] = React.useState<ResolvedItem[]>([]);
  const { pendingQueue, setPendingQueue } = useAppStore();

  // Keep a ref so event handlers always see the latest queue without re-subscribing
  const pendingQueueRef = React.useRef<PendingModeration[]>(pendingQueue);
  React.useEffect(() => { pendingQueueRef.current = pendingQueue; }, [pendingQueue]);

  React.useEffect(() => {
    // Reactive data source using polling to keep badges and the queue instantly in sync
    const fetchStats = () => {
      GetPendingModerationStats().then((stats) => {
        if (!stats?.items) {
          setPendingQueue([]);
          return;
        }
        const queue: PendingModeration[] = stats.items.map((item: any) => ({
          id: item.msg_sign,
          ts: item.ts ?? 0,
          content: item.content ?? '',
          reason: item.reason ?? (item.is_image ? 'Image attached' : ''),
          approved: item.approved,
          rejected: item.rejected,
          totalMods: item.approved + item.rejected + item.awaiting,
          ackCount: item.approved + item.rejected,
          awaitingMods: item.awaiting,
        }));
        setPendingQueue(queue);
      }).catch(() => {});
    };

    fetchStats();
    // Start polling interval
    const intervalId = setInterval(fetchStats, 5000);

    // Live updates from cron
    const unsubCronStatus = EventsOn("cron_status_update", (queue: PendingModeration[]) => {
      setPendingQueue(queue || []);
    });

    // Toast + retain finalized items in the queue with a status badge
    const unsubFinalized = EventsOn("moderation_finalized", (event: { status: string; id: string }) => {
      const finalStatus = event.status === "approved" ? 'approved' : 'rejected';

      if (finalStatus === 'approved') {
        toast.success(`Message Approved (${event.id.substring(0, 8)}...)`);
      } else {
        toast.error(`Message Rejected (${event.id.substring(0, 8)}...)`);
      }

      // Find the item in the current queue snapshot
      const source = pendingQueueRef.current.find(q => q.id === event.id);
      const resolved: ResolvedItem = source
        ? { ...source, finalStatus }
        : { id: event.id, ts: 0, content: '', reason: '', approved: 0, rejected: 0, totalMods: 0, ackCount: 0, awaitingMods: 0, finalStatus };

      setResolvedItems(prev => [...prev.filter(r => r.id !== event.id), resolved]);

      // Auto-dismiss after 30 seconds
      setTimeout(() => {
        setResolvedItems(prev => prev.filter(r => r.id !== event.id));
      }, 30_000);
    });

    return () => {
      clearInterval(intervalId);
      unsubCronStatus();
      unsubFinalized();
    };
  }, []);
  React.useEffect(() => {
    logger.debug('[Menubar] Component mounted.');
    async function fetchMods() {
      try {
        const keys = await GetOnlineMods();
        logger.debug('[Menubar] Received keys:', keys);
        const resolved = await Promise.all(
          keys.map(async (key) => {
            const alias = await GenerateAlias(key);
            const avatarSvg = await GenerateAvatar(key);
            logger.debug(`[Menubar] Processed mod: ${alias}`);
            return { key, alias, avatarSvg };
          })
        );
        setMods(resolved);
        logger.info('[Menubar] Mods loaded successfully.');
      } catch (err) {
        logger.error('[Menubar] Failed to load online mods:', err);
      }
    }

    fetchMods();

    // Only return cleanup function, not async code
    return () => logger.debug('[Menubar] Component unmounted.');
  }, []);

  return (
    <div className="relative w-full h-full flex flex-col z-50 pt-3 pb-0 pl-4 pr-0 gap-4 bg-transparent border-l border-border/5 overflow-hidden box-border">
      <ComingSoonDialog open={dialogOpen} onClose={() => {
        logger.info('[ComingSoonDialog] Closed.');
        setDialogOpen(false)
      }} />
      {/* Scrollable content */}
      <div className="flex-1 overflow-y-auto overflow-x-hidden flex flex-col w-full gap-4 pr-0 soft-scrollbar box-border">
        {/* Moderators Floating Card */}
        <div className="bg-card/70 backdrop-blur-xl shadow-lg border border-border/50 rounded-2xl p-4 w-full flex flex-col gap-3 relative overflow-hidden box-border">
          <h3 className="text-xs font-bold uppercase tracking-wider text-muted-foreground/80 mb-1 flex items-center gap-2">
            Moderators
          </h3>
          <div className="flex flex-col gap-2.5 w-full">
          {mods.map(({ key, alias, avatarSvg }) => (
            <div key={key} className="flex items-center justify-start space-x-3 py-1">
              {avatarSvg && avatarSvg !== "unknown" ? (
                <img
                  src={`data:image/svg+xml;base64,${avatarSvg}`}
                  alt="avatar"
                  className="w-10 h-10 rounded-xl"
                />
              ) : (
                <div className="w-10 h-10 bg-libr-accent1 rounded-full flex items-center justify-center">
                  <span className="text-white text-sm font-medium">
                    {alias.charAt(0).toUpperCase()}
                  </span>
                </div>
              )}
              <span className="text-sm font-medium text-foreground">{alias}</span>
            </div>
          ))}
          </div>
        </div>

        {/* Moderation Queue Floating Card */}
        <div className="bg-card/70 backdrop-blur-xl shadow-lg border border-border/50 rounded-2xl p-4 w-full flex flex-col gap-3 relative overflow-hidden box-border">
          <button
            onClick={() => setQueueOpen(prev => !prev)}
            className="text-left w-full flex items-center hover:opacity-80 transition-opacity outline-none"
          >
            <div className="flex items-center justify-between w-full">
              <h3 className="text-xs font-bold uppercase tracking-wider text-muted-foreground/80 flex items-center gap-1.5">
                <ChevronDown
                  className={`w-3.5 h-3.5 text-muted-foreground/60 transition-transform duration-200 ${queueOpen ? '' : '-rotate-90'}`}
                />
                Moderation Queue
              </h3>
              {(pendingQueue.length + resolvedItems.length) > 0 && (
                <span className="bg-libr-accent1/20 border border-libr-accent1/30 text-libr-accent1 text-[10px] font-bold px-2 py-0.5 rounded-full">
                  {pendingQueue.length + resolvedItems.length}
                </span>
              )}
            </div>
          </button>

          {queueOpen && (
            <div className="flex flex-col gap-2 w-full mt-2">
            {pendingQueue.length === 0 && resolvedItems.length === 0 ? (
              <p className="text-xs text-muted-foreground pl-1">No items pending.</p>
            ) : (
              <>
                {/* Active pending items */}
                {pendingQueue.filter(item => !resolvedItems.some(r => r.id === item.id)).map((item) => {
                  const isOpen = expandedItem === item.id;
                  const isImage = item.reason === 'Image attached';
                  const isReport = !isImage && !!item.reason;

                  const rawText = item.content
                    ? item.content
                        .replace(/<HEAD>[\s\S]*?<\/HEAD>/gi, '')
                        .replace(/<BODY>([\s\S]*?)<\/BODY>/gi, '$1')
                        .replace(/<[^>]+>/g, '')
                        .trim()
                    : '';
                  const preview = rawText.length > 0
                    ? rawText.substring(0, 40) + (rawText.length > 40 ? '…' : '')
                    : null;

                  const TypeIcon = isImage ? ImageIcon : isReport ? Flag : FileText;

                  return (
                    <div
                      key={item.id}
                      className="bg-muted/30 rounded-lg border border-border/50 overflow-hidden cursor-pointer hover:border-libr-accent1/40 transition-colors"
                      onClick={() => setExpandedItem(isOpen ? null : item.id)}
                    >
                      {/* Collapsed row */}
                      <div className="flex items-center justify-between p-3 text-xs gap-2">
                        <div className="flex items-center gap-1.5 min-w-0">
                          <ChevronRight
                            className={`w-3 h-3 text-muted-foreground flex-shrink-0 transition-transform duration-150 ${isOpen ? 'rotate-90' : ''}`}
                          />
                          <TypeIcon className="w-3 h-3 text-libr-accent1 flex-shrink-0" />
                          <span className="font-semibold text-libr-accent1 truncate">
                            {preview ?? (isImage ? 'Image message' : isReport ? item.reason : 'Pending message')}
                          </span>
                        </div>
                        <span className="text-muted-foreground/70 flex-shrink-0 tabular-nums">
                          {item.ts > 0
                            ? new Date(item.ts * 1000).toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' })
                            : '—'}
                        </span>
                      </div>

                      {/* Expanded details */}
                      {isOpen && (
                        <div className="px-3 pb-3 pt-1 border-t border-border/30 text-xs flex flex-col gap-2">
                          {isImage ? (
                            <div className="flex items-center gap-1.5 text-muted-foreground italic">
                              <ImageIcon className="w-3 h-3" />
                              <span>Image attachment</span>
                            </div>
                          ) : rawText.length > 0 ? (
                            <div className="text-foreground/80 leading-relaxed break-words bg-background/40 rounded-md px-2.5 py-2 border border-border/30">
                              {rawText.substring(0, 200)}{rawText.length > 200 ? '…' : ''}
                            </div>
                          ) : null}

                          {isReport && (
                            <div className="flex items-center gap-1">
                              <Flag className="w-2.5 h-2.5 text-yellow-400" />
                              <span className="text-yellow-400/80">{item.reason}</span>
                            </div>
                          )}

                          {(() => {
                            const tot = item.approved + item.rejected + item.awaitingMods;
                            const wA = tot === 0 ? 33.33 : (item.approved / tot) * 100;
                            const wR = tot === 0 ? 33.33 : (item.rejected / tot) * 100;
                            const wW = tot === 0 ? 33.33 : (item.awaitingMods / tot) * 100;
                            return (
                              <div className="flex flex-col mt-2 mb-1 w-full">
                                {/* Floating numbers */}
                                <div className="flex w-full gap-0.5 text-[10px] font-bold leading-none mb-1">
                                  <div className={`flex justify-center transition-all duration-300 ${item.approved > 0 ? 'text-teal-400' : 'text-transparent'}`} style={{ width: `${wA}%` }}>{item.approved}</div>
                                  <div className={`flex justify-center transition-all duration-300 ${item.rejected > 0 ? 'text-rose-500' : 'text-transparent'}`} style={{ width: `${wR}%` }}>{item.rejected}</div>
                                  <div className={`flex justify-center transition-all duration-300 ${item.awaitingMods > 0 ? 'text-amber-400' : 'text-transparent'}`} style={{ width: `${wW}%` }}>{item.awaitingMods}</div>
                                </div>
                                {/* Bar */}
                                <div className="flex w-full h-[3px] rounded-full bg-muted/10 gap-0.5 overflow-hidden">
                                  <div className={`h-full transition-all duration-300 ${item.approved > 0 ? 'bg-teal-400 shadow-[0_0_8px_rgba(45,212,191,0.8)]' : 'bg-muted/20'}`} style={{ width: `${wA}%` }} />
                                  <div className={`h-full transition-all duration-300 ${item.rejected > 0 ? 'bg-rose-500 shadow-[0_0_8px_rgba(244,63,94,0.8)]' : 'bg-muted/20'}`} style={{ width: `${wR}%` }} />
                                  <div className={`h-full transition-all duration-300 ${item.awaitingMods > 0 ? 'bg-amber-400 shadow-[0_0_8px_rgba(251,191,36,0.8)]' : 'bg-muted/20'}`} style={{ width: `${wW}%` }} />
                                </div>
                                {/* Micro-text */}
                                <div className="flex items-center justify-center text-[10px] text-muted-foreground/50 font-medium lowercase tracking-wide mt-2">
                                  <span className={item.approved > 0 ? 'text-teal-400/80 font-semibold' : ''}>{item.approved} approved</span>
                                  <span className="mx-1.5 opacity-40">·</span>
                                  <span className={item.rejected > 0 ? 'text-rose-500/80 font-semibold' : ''}>{item.rejected} rejected</span>
                                  <span className="mx-1.5 opacity-40">·</span>
                                  <span className={item.awaitingMods > 0 ? 'text-amber-400/80 font-semibold' : ''}>{item.awaitingMods} waiting</span>
                                </div>
                              </div>
                            );
                          })()}

                          <div className="font-mono text-muted-foreground/50 text-[10px] break-all">
                            {item.id.substring(0, 24)}…
                          </div>
                        </div>
                      )}
                    </div>
                  );
                })}

                {/* Finalized items — stay visible until dismissed or auto-cleared */}
                {resolvedItems.map((item) => {
                  const isApproved = item.finalStatus === 'approved';
                  const isOpen = expandedItem === item.id;
                  const isImage = item.reason === 'Image attached';
                  const isReport = !isImage && !!item.reason;
                  const borderColor = isApproved ? 'border-green-500/40' : 'border-red-500/40';
                  const bgColor = isApproved ? 'bg-green-500/5' : 'bg-red-500/5';

                  const rawText = item.content
                    ? item.content
                        .replace(/<HEAD>[\s\S]*?<\/HEAD>/gi, '')
                        .replace(/<BODY>([\s\S]*?)<\/BODY>/gi, '$1')
                        .replace(/<[^>]+>/g, '')
                        .trim()
                    : '';
                  const preview = rawText.length > 0
                    ? rawText.substring(0, 40) + (rawText.length > 40 ? '…' : '')
                    : null;

                  const StatusIcon = isApproved ? CheckCircle2 : XCircle;
                  const statusColor = isApproved ? 'text-green-400' : 'text-red-400';

                  return (
                    <div
                      key={`resolved-${item.id}`}
                      className={`rounded-lg border ${borderColor} ${bgColor} overflow-hidden`}
                    >
                      {/* Header row */}
                      <div
                        className="flex items-center justify-between p-3 text-xs gap-2 cursor-pointer"
                        onClick={() => setExpandedItem(isOpen ? null : item.id)}
                      >
                        <div className="flex items-center gap-1.5 min-w-0">
                          <ChevronRight
                            className={`w-3 h-3 text-muted-foreground flex-shrink-0 transition-transform duration-150 ${isOpen ? 'rotate-90' : ''}`}
                          />
                          <StatusIcon className={`w-3.5 h-3.5 ${statusColor} flex-shrink-0`} />
                          <span className={`font-semibold ${statusColor} truncate`}>
                            {preview ?? (isImage ? 'Image message' : isReport ? item.reason : 'Message')}
                          </span>
                        </div>
                        <div className="flex items-center gap-1.5 flex-shrink-0">
                          <span className={`font-semibold uppercase text-[10px] tracking-wide ${statusColor}`}>
                            {item.finalStatus}
                          </span>
                          <button
                            onClick={(e) => {
                              e.stopPropagation();
                              setResolvedItems(prev => prev.filter(r => r.id !== item.id));
                            }}
                            className="text-muted-foreground/50 hover:text-muted-foreground transition-colors"
                          >
                            <X className="w-3 h-3" />
                          </button>
                        </div>
                      </div>

                      {/* Expanded details */}
                      {isOpen && (
                        <div className="px-3 pb-3 pt-1 border-t border-border/20 text-xs flex flex-col gap-2">
                          {isImage ? (
                            <div className="flex items-center gap-1.5 text-muted-foreground italic">
                              <ImageIcon className="w-3 h-3" />
                              <span>Image attachment</span>
                            </div>
                          ) : rawText.length > 0 ? (
                            <div className="text-foreground/70 leading-relaxed break-words bg-background/30 rounded-md px-2.5 py-2 border border-border/20">
                              {rawText.substring(0, 200)}{rawText.length > 200 ? '…' : ''}
                            </div>
                          ) : null}

                          {isReport && (
                            <div className="flex items-center gap-1">
                              <Flag className="w-2.5 h-2.5 text-yellow-400" />
                              <span className="text-yellow-400/80">{item.reason}</span>
                            </div>
                          )}

                          <div className="font-mono text-muted-foreground/40 text-[10px] break-all">
                            {item.id.substring(0, 24)}…
                          </div>
                        </div>
                      )}
                    </div>
                  );
                })}
              </>
            )}
          </div>
        )}
        </div>
      </div>

      {/* Embedded Options Section */}
      <div className="flex flex-col w-full gap-1 pl-2 pr-0 pb-2 mt-auto border-t border-border/5 pt-4 relative z-0">
        <button
          onClick={() => {
            logger.info('[Menubar] EULA link clicked.');
            BrowserOpenURL("https://libr-ashen.vercel.app/eula");
          }}
          className="flex items-center space-x-3 w-full px-3 py-2 text-muted-foreground/60 hover:text-foreground/90 hover:bg-foreground/5 rounded-xl transition-all text-sm font-medium outline-none"
        >
          <Copyright className="w-4 h-4 text-muted-foreground/50" />
          <span>License & Agreement</span>
        </button>

        <button
          onClick={() => {
            logger.info('[Menubar] Feedback link clicked.');
            BrowserOpenURL("https://forms.gle/Uchqc6Z49aoJwjvZ9");
          }}
          className="flex items-center space-x-3 w-full px-3 py-2 text-muted-foreground/60 hover:text-foreground/90 hover:bg-foreground/5 rounded-xl transition-all text-sm font-medium outline-none"
        >
          <PencilLine className="w-4 h-4 text-muted-foreground/50" />
          <span>Feedback</span>
        </button>

        <button
          onClick={() => {
            logger.info('[Menubar] Website link clicked.');
            BrowserOpenURL("https://libr-ashen.vercel.app/")
          }}
          className="flex items-center space-x-3 w-full px-3 py-2 text-muted-foreground/60 hover:text-foreground/90 hover:bg-foreground/5 rounded-xl transition-all text-sm font-medium outline-none"
        >
          <Globe className="w-4 h-4 text-muted-foreground/50" />
          <span>Visit Website</span>
        </button>

        <button
          onClick={() => {
            logger.info('[Menubar] Open host database dialog.');
            setDialogOpen(true)
          }}
          className="flex items-center space-x-3 w-full px-3 py-2 text-muted-foreground/60 hover:text-foreground/90 hover:bg-foreground/5 rounded-xl transition-all text-sm font-medium outline-none"
        >
          <Database className="w-4 h-4 text-muted-foreground/50" />
          <span>Host a database</span>
        </button>
      </div>
    </div>
  );
};
