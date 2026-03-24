import React, { useEffect, useState, useRef, useCallback } from 'react';
import { StyleSheet, View, Text, FlatList, TouchableOpacity, StatusBar, Modal } from 'react-native';
import AsyncStorage from '@react-native-async-storage/async-storage';
import { SafeAreaView, useSafeAreaInsets } from 'react-native-safe-area-context';
import { useRouter } from 'expo-router';
import { ArrowLeft, Flag, CheckCircle, Clock, XCircle, Bell } from 'lucide-react-native';
import { useAppStore } from '@/store/useAppStore';
import LibrCore, { RetMsgCert } from '@/modules/LibrCore';
import { Image } from 'expo-image';

type QueueItem = RetMsgCert & {
    placeholder?: boolean;
};

const C = {
    bg: '#0a0f1c',
    card: '#0f1625',
    border: '#1e2b3c',
    teal: '#1fa4a9',
    green: '#00ffd0',
    red: '#ff4444',
    text: '#ffffff',
    muted: '#8b9bb4',
    amber: '#ffb300',
};

// Styled in-app notification banner
function NotificationBanner({ visible, type, message, onDismiss }: { visible: boolean; type: 'approved' | 'rejected'; message: string; onDismiss: () => void }) {
    if (!visible) return null;
    const isApproved = type === 'approved';
    return (
        <Modal visible={visible} transparent animationType="slide" onRequestClose={onDismiss}>
            <TouchableOpacity style={{ flex: 1 }} activeOpacity={1} onPress={onDismiss}>
                <View style={{
                    position: 'absolute', top: 60, left: 16, right: 16,
                    backgroundColor: C.card, borderRadius: 16, padding: 16,
                    borderWidth: 1, borderColor: isApproved ? C.green + '40' : C.red + '40',
                    flexDirection: 'row', alignItems: 'center', gap: 12,
                    shadowColor: '#000', shadowOffset: { width: 0, height: 4 }, shadowOpacity: 0.3, shadowRadius: 8, elevation: 10,
                }}>
                    <View style={{
                        width: 40, height: 40, borderRadius: 20,
                        backgroundColor: isApproved ? C.green + '20' : C.red + '20',
                        alignItems: 'center', justifyContent: 'center',
                    }}>
                        {isApproved ? <CheckCircle size={20} color={C.green} /> : <XCircle size={20} color={C.red} />}
                    </View>
                    <View style={{ flex: 1 }}>
                        <Text style={{ color: isApproved ? C.green : C.red, fontWeight: '700', fontSize: 15 }}>
                            {isApproved ? 'Report Approved' : 'Report Rejected'}
                        </Text>
                        <Text style={{ color: C.muted, fontSize: 13, marginTop: 2 }}>{message}</Text>
                    </View>
                </View>
            </TouchableOpacity>
        </Modal>
    );
}

export default function ModerationQueueScreen() {
    const router = useRouter();
    const { state } = useAppStore();
    const insets = useSafeAreaInsets();
    const [queueMessages, setQueueMessages] = useState<QueueItem[]>([]);
    const [pendingStatus, setPendingStatus] = useState<Record<string, { total: number, approved: number, rejected: number }>>({});
    const [loading, setLoading] = useState(true);
    const [notification, setNotification] = useState<{ visible: boolean; type: 'approved' | 'rejected'; message: string }>({ visible: false, type: 'approved', message: '' });
    const [dismissedSigns, setDismissedSigns] = useState<Set<string>>(new Set());
    // Track finalized reports: sign -> 'approved' | 'rejected'
    const [resolvedSigns, setResolvedSigns] = useState<Record<string, 'approved' | 'rejected'>>({});

    // Cache reported messages so they persist even after deletion from feed
    const cachedMessagesRef = useRef<Map<string, RetMsgCert>>(new Map());
    const prevStatusRef = useRef<Record<string, { total: number, approved: number, rejected: number }>>({});
    const autoDismissTimers = useRef<Set<string>>(new Set());

    // Show a styled notification that auto-dismisses after 5 seconds
    const showNotification = useCallback((type: 'approved' | 'rejected', message: string) => {
        setNotification({ visible: true, type, message });
        setTimeout(() => setNotification(prev => ({ ...prev, visible: false })), 5000);
    }, []);

    // Dismiss a completed report — remove from state + AsyncStorage
    const dismissReport = useCallback((sign: string) => {
        setDismissedSigns(prev => new Set(prev).add(sign));
        setQueueMessages(prev => prev.filter(m => m.sign !== sign));
        // Clean from AsyncStorage
        AsyncStorage.getItem('@libr_reported_signs').then((data: string | null) => {
            if (data) {
                const arr: string[] = JSON.parse(data).filter((s: string) => s !== sign);
                AsyncStorage.setItem('@libr_reported_signs', JSON.stringify(arr));
            }
        }).catch(() => {});
        AsyncStorage.getItem('@libr_reported_messages').then((data: string | null) => {
            if (data) {
                const cache = JSON.parse(data);
                delete cache[sign];
                AsyncStorage.setItem('@libr_reported_messages', JSON.stringify(cache));
            }
        }).catch(() => {});
        cachedMessagesRef.current.delete(sign);
    }, []);

    // Fetch messages and cache them
    useEffect(() => {
        let cancelled = false;

        const fetchDetails = async () => {
            const pendingSigns = Object.keys(pendingStatus);
            if (state.reportedSigns.size === 0 && pendingSigns.length === 0) {
                if (!cancelled) {
                    setQueueMessages([]);
                    setLoading(false);
                }
                return;
            }

            setLoading(true);
            try {
                // Hydrate from AsyncStorage on first load
                if (cachedMessagesRef.current.size === 0) {
                    try {
                        const stored = await AsyncStorage.getItem('@libr_reported_messages');
                        if (stored) {
                            const parsed: Record<string, RetMsgCert> = JSON.parse(stored);
                            for (const [sign, cert] of Object.entries(parsed)) {
                                cachedMessagesRef.current.set(sign, cert);
                            }
                        }
                    } catch { }
                }

                // Update cache with any messages from the feed
                for (const msg of state.messages) {
                    if (state.reportedSigns.has(msg.sign) || pendingSigns.includes(msg.sign)) {
                        cachedMessagesRef.current.set(msg.sign, msg);
                    }
                }

                // For any pending/reported signs not in cache, try to fetch them from DHT
                const targetSigns = Array.from(new Set([...pendingSigns, ...Array.from(state.reportedSigns)]));
                const missingSigns = targetSigns.filter(s => !cachedMessagesRef.current.has(s));
                for (const sign of missingSigns) {
                    try {
                        if (typeof (LibrCore as any).fetchMessageBySign === 'function') {
                            const raw = await (LibrCore as any).fetchMessageBySign(sign);
                            if (raw && !raw.startsWith('error:')) {
                                cachedMessagesRef.current.set(sign, JSON.parse(raw));
                            }
                        }
                    } catch {
                        // Ignore individual fetch errors
                    }
                }

                // Detect resolved reports from live feed
                const newResolved: Record<string, 'approved' | 'rejected'> = {};
                for (const msg of state.messages) {
                    if (state.reportedSigns.has(msg.sign) && msg.deleted === '1') {
                        newResolved[msg.sign] = 'approved';
                    }
                }
                if (Object.keys(newResolved).length > 0) {
                    setResolvedSigns(prev => ({ ...prev, ...newResolved }));
                }

                // Build display list from the pending moderation queue.
                const results: QueueItem[] = [];
                for (const sign of pendingSigns) {
                    if (dismissedSigns.has(sign)) continue;
                    const cached = cachedMessagesRef.current.get(sign);
                    if (cached) {
                        results.push(cached);
                    } else {
                        results.push({
                            public_key: '',
                            sign,
                            msg: {
                                content: '',
                                ts: 0,
                            },
                            mod_certs: [],
                            deleted: '0',
                            placeholder: true,
                        });
                    }
                }

                if (!cancelled) setQueueMessages(results);
            } catch (err) {
                console.warn('Failed to fetch reported messages details:', err);
            } finally {
                if (!cancelled) setLoading(false);
            }
        };

        fetchDetails();
        return () => { cancelled = true; };
    }, [state.reportedSigns, state.messages, dismissedSigns, pendingStatus]);

    // Poll pendingStatus every 5 seconds (mirrors desktop's setInterval pattern)
    useEffect(() => {
        const pollStatus = async () => {
            if (typeof (LibrCore as any).getPendingReports !== 'function') return;
            try {
                const rawStatus = await (LibrCore as any).getPendingReports();
                if (rawStatus && !rawStatus.startsWith('error:')) {
                    const newPending = JSON.parse(rawStatus);
                    const prev = prevStatusRef.current;

                    if (Object.keys(prev).length > 0) {
                        // Detect signs that DISAPPEARED from pendingStatus = finalized by cron
                        for (const sign in prev) {
                            if (!state.reportedSigns.has(sign)) continue;
                            if (!(sign in newPending)) {
                                // Was pending before, gone now → finalized
                                // INVERTED: rejected (status '0') = mods want message deleted = report approved
                                const reportApproved = prev[sign].rejected > prev[sign].approved;
                                const finalStatus = reportApproved ? 'approved' : 'rejected';
                                setResolvedSigns(r => ({ ...r, [sign]: finalStatus as 'approved' | 'rejected' }));
                                showNotification(finalStatus as 'approved' | 'rejected',
                                    reportApproved
                                        ? 'Your report was upheld — the message has been removed.'
                                        : 'Your report was dismissed — the message will stay.'
                                );
                                // Auto-dismiss after 30 seconds
                                if (!autoDismissTimers.current.has(sign)) {
                                    autoDismissTimers.current.add(sign);
                                    setTimeout(() => dismissReport(sign), 30_000);
                                }
                            }
                        }

                        // Detect quorum threshold crossings for still-pending items
                        for (const sign in newPending) {
                            if (!state.reportedSigns.has(sign)) continue;
                            const pOld = prev[sign];
                            const pNew = newPending[sign];
                            if (pOld && pNew && pNew.total > 0) {
                                const threshold = Math.floor(pNew.total / 2);
                                // INVERTED: rejected (status '0') = in favor of report
                                if (pOld.rejected <= threshold && pNew.rejected > threshold) {
                                    setResolvedSigns(r => ({ ...r, [sign]: 'approved' }));
                                    showNotification('approved', 'Your report was upheld — the message has been removed.');
                                    if (!autoDismissTimers.current.has(sign)) {
                                        autoDismissTimers.current.add(sign);
                                        setTimeout(() => dismissReport(sign), 30_000);
                                    }
                                } else if (pOld.approved <= threshold && pNew.approved > threshold) {
                                    setResolvedSigns(r => ({ ...r, [sign]: 'rejected' }));
                                    showNotification('rejected', 'Your report was dismissed — the message will stay.');
                                    if (!autoDismissTimers.current.has(sign)) {
                                        autoDismissTimers.current.add(sign);
                                        setTimeout(() => dismissReport(sign), 30_000);
                                    }
                                }
                            }
                        }
                    }

                    prevStatusRef.current = newPending;
                    setPendingStatus(newPending);
                }
            } catch { }
        };

        pollStatus();
        const intervalId = setInterval(pollStatus, 5000);
        return () => clearInterval(intervalId);
    }, [state.reportedSigns, showNotification, dismissReport]);

    const renderItem = ({ item }: { item: QueueItem }) => {
        // Resolution detection: check resolvedSigns map first, then fallback to item.deleted
        const pStatus = pendingStatus[item.sign];

        // Extract plain content
        let rawContent = item.msg.content;
        const matchBody = rawContent.match(/<BODY>(.*?)<\/BODY>/s);
        if (matchBody) rawContent = matchBody[1].trim();
        const plainContent = rawContent.replace(/<\/?[^>]+(>|$)/g, '').replace(/&amp;/g, '&').replace(/&lt;/g, '<').replace(/&gt;/g, '>').replace(/&quot;/g, '"').replace(/&#39;/g, "'").trim();

        // Extract images
        const imgSrcs: string[] = [];
        const imgRegex = /<img[^>]+src="([^">]+)"/g;
        let match;
        while ((match = imgRegex.exec(item.msg.content)) !== null) {
            imgSrcs.push(match[1]);
        }

        return (
            <View style={styles.card}>
                <View style={{ flexDirection: 'row', alignItems: 'flex-start' }}>
                    <Text style={styles.labelSpan}>Message: </Text>
                    {plainContent ? (
                        <Text style={styles.contentText} numberOfLines={10}>
                            {plainContent}
                        </Text>
                    ) : (
                        <Text style={[styles.contentText, { fontStyle: 'italic', color: C.muted }]}>
                            {item.placeholder ? `Pending moderation item\n${item.sign.slice(0, 18)}...` : '[No text]'}
                        </Text>
                    )}
                </View>

                {imgSrcs.length > 0 && (
                    <View style={styles.imageGallery}>
                        {imgSrcs.map((src, i) => (
                            <Image
                                key={i}
                                source={src}
                                style={styles.reportImage}
                                contentFit="cover"
                            />
                        ))}
                    </View>
                )}

                <View style={styles.statusRow}>
                    <View style={{ gap: 8, paddingBottom: 8 }}>
                        <View style={styles.statusBadge}>
                            <Clock size={14} color={C.amber} />
                            <Text style={[styles.statusText, { color: C.amber }]}>
                                {item.msg.content.includes('<img') ? 'Image Under Review' : 'Under Review'}
                            </Text>
                        </View>
                        {pStatus && pStatus.total > 0 && (
                            <View style={{ flexDirection: 'row', gap: 12, paddingHorizontal: 10 }}>
                                <Text style={{ fontSize: 13, color: C.green, fontWeight: '600' }}>Approved: {pStatus.approved}</Text>
                                <Text style={{ fontSize: 13, color: C.red, fontWeight: '600' }}>Rejected: {pStatus.rejected}</Text>
                                <Text style={{ fontSize: 13, color: C.amber, fontWeight: '600' }}>Waiting: {Math.max(0, pStatus.total - pStatus.approved - pStatus.rejected)}</Text>
                            </View>
                        )}
                    </View>
                </View>
            </View>
        );
    };

    return (
        <SafeAreaView style={styles.safeArea} edges={['left', 'right', 'bottom']}>
            <StatusBar barStyle="light-content" backgroundColor={C.bg} />
            {/* Header */}
            <View style={[styles.header, { paddingTop: Math.max(insets.top, 10) }]}>
                <TouchableOpacity style={styles.backBtn} onPress={() => router.back()}>
                    <ArrowLeft size={24} color={C.text} />
                </TouchableOpacity>
                <Text style={styles.title}>Moderation Queue</Text>
                <View style={{ width: 40 }} />
            </View>

            {loading ? (
                <View style={styles.centerContainer}>
                    <Text style={styles.emptyText}>Loading...</Text>
                </View>
            ) : queueMessages.length === 0 ? (
                <View style={styles.centerContainer}>
                    <Bell size={48} color={C.border} style={{ marginBottom: 16 }} />
                    <Text style={styles.emptyText}>No items pending moderation right now.</Text>
                </View>
            ) : (
                <FlatList
                    data={queueMessages}
                    keyExtractor={(item) => item.sign}
                    renderItem={renderItem}
                    contentContainerStyle={styles.listContainer}
                />
            )}

            <NotificationBanner
                visible={notification.visible}
                type={notification.type}
                message={notification.message}
                onDismiss={() => setNotification(prev => ({ ...prev, visible: false }))}
            />
        </SafeAreaView>
    );
}

const styles = StyleSheet.create({
    safeArea: {
        flex: 1,
        backgroundColor: C.bg,
    },
    header: {
        flexDirection: 'row',
        alignItems: 'center',
        justifyContent: 'space-between',
        paddingHorizontal: 16,
        paddingVertical: 12,
        borderBottomWidth: 1,
        borderBottomColor: C.border,
    },
    backBtn: {
        padding: 8,
    },
    title: {
        fontSize: 18,
        fontWeight: '700',
        color: C.text,
    },
    centerContainer: {
        flex: 1,
        justifyContent: 'center',
        alignItems: 'center',
        padding: 20,
    },
    emptyText: {
        color: C.muted,
        fontSize: 16,
        textAlign: 'center',
    },
    listContainer: {
        padding: 16,
        gap: 16,
    },
    card: {
        backgroundColor: C.card,
        borderRadius: 20,
        padding: 20,
        borderWidth: 1,
        borderColor: C.border,
        shadowColor: '#000',
        shadowOffset: { width: 0, height: 2 },
        shadowOpacity: 0.1,
        shadowRadius: 4,
        elevation: 2,
    },
    labelSpan: {
        color: C.muted,
        fontWeight: '700',
        fontSize: 14,
        marginRight: 4,
    },
    contentText: {
        color: C.text,
        fontSize: 14,
        lineHeight: 20,
        flex: 1,
    },
    statusRow: {
        flexDirection: 'row',
        justifyContent: 'space-between',
        marginTop: 16,
        paddingTop: 16,
        borderTopWidth: 1,
        borderTopColor: C.border + '50',
    },
    statusBadge: {
        flexDirection: 'row',
        alignItems: 'center',
        gap: 6,
        backgroundColor: C.bg,
        paddingHorizontal: 10,
        paddingVertical: 4,
        borderRadius: 12,
    },
    statusText: {
        fontSize: 12,
        fontWeight: '600',
    },
    actionTakenContainer: {
        flexDirection: 'column',
        alignItems: 'flex-start',
        backgroundColor: C.bg,
        borderRadius: 12,
        overflow: 'hidden',
        paddingHorizontal: 10,
        paddingVertical: 6,
    },
    modDivider: {
        height: 1,
        width: '100%',
        backgroundColor: C.border,
    },
    modList: {
        paddingHorizontal: 10,
        paddingVertical: 6,
        flexDirection: 'row',
        alignItems: 'center',
        gap: 8,
    },
    modListTitle: {
        fontSize: 11,
        color: C.muted,
        fontWeight: '500',
    },
    modAvatarsContainer: {
        flexDirection: 'row',
        alignItems: 'center',
    },
    modAvatar: {
        width: 20,
        height: 20,
        borderRadius: 10,
        justifyContent: 'center',
        alignItems: 'center',
        marginLeft: -6,
        borderWidth: 1.5,
        borderColor: C.bg,
    },
    modAvatarText: {
        color: '#fff',
        fontSize: 9,
        fontWeight: 'bold',
    },
    modCountText: {
        fontSize: 11,
        color: C.muted,
        marginLeft: 8,
        fontWeight: '600',
    },
    imageGallery: {
        flexDirection: 'row',
        flexWrap: 'wrap',
        gap: 8,
        marginTop: 12,
    },
    reportImage: {
        width: 100,
        height: 100,
        borderRadius: 12,
        backgroundColor: '#1a2235',
        borderWidth: 1,
        borderColor: C.border,
    },
});
