import React, { useEffect, useState } from 'react';
import { StyleSheet, View, Text, FlatList, TouchableOpacity, StatusBar } from 'react-native';
import { SafeAreaView, useSafeAreaInsets } from 'react-native-safe-area-context';
import { useRouter } from 'expo-router';
import { ArrowLeft, Flag, CheckCircle, Clock } from 'lucide-react-native';
import { useAppStore } from '@/store/useAppStore';
import LibrCore, { RetMsgCert } from '@/modules/LibrCore';

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

export default function MyReportsScreen() {
    const router = useRouter();
    const { state } = useAppStore();
    const insets = useSafeAreaInsets();
    const [reportedMessages, setReportedMessages] = useState<RetMsgCert[]>([]);
    const [pendingStatus, setPendingStatus] = useState<Record<string, { total: number, approved: number, rejected: number }>>({});
    const [loading, setLoading] = useState(true);

    useEffect(() => {
        let cancelled = false;

        const fetchDetails = async () => {
            if (state.reportedSigns.size === 0) {
                if (!cancelled) {
                    setReportedMessages([]);
                    setLoading(false);
                }
                return;
            }

            setLoading(true);
            try {
                const results: RetMsgCert[] = [];
                // We iterate through all fetched messages first as an optimization
                // and keep track of ones we found.
                const foundSigns = new Set<string>();

                for (const msg of state.messages) {
                    if (state.reportedSigns.has(msg.sign)) {
                        results.push(msg);
                        foundSigns.add(msg.sign);
                    }
                }

                // For any reported signs not in the current feed, try to fetch them from DHT
                const missingSigns = Array.from(state.reportedSigns).filter(s => !foundSigns.has(s));
                for (const sign of missingSigns) {
                    try {
                        // Try to fetch individual message (assuming core supports it, otherwise it skips)
                        if (typeof (LibrCore as any).fetchMessageBySign === 'function') {
                            const raw = await (LibrCore as any).fetchMessageBySign(sign);
                            if (raw && !raw.startsWith('error:')) {
                                results.push(JSON.parse(raw));
                            }
                        }
                    } catch {
                        // Ignore individual fetch errors
                    }
                }

                if (typeof (LibrCore as any).getPendingReports === 'function') {
                    try {
                        const rawStatus = await (LibrCore as any).getPendingReports();
                        if (rawStatus && !rawStatus.startsWith('error:')) {
                            setPendingStatus(JSON.parse(rawStatus));
                        }
                    } catch { }
                }

                if (!cancelled) setReportedMessages(results);
            } catch (err) {
                console.warn('Failed to fetch reported messages details:', err);
            } finally {
                if (!cancelled) setLoading(false);
            }
        };

        fetchDetails();
        return () => { cancelled = true; };
    }, [state.reportedSigns, state.messages]);

    const renderItem = ({ item }: { item: RetMsgCert }) => {
        const isApproved = item.deleted === '1';

        // Extract plain content
        let rawContent = item.msg.content;
        const matchBody = rawContent.match(/<BODY>(.*?)<\/BODY>/s);
        if (matchBody) rawContent = matchBody[1].trim();
        const plainContent = rawContent.replace(/<\/?[^>]+(>|$)/g, '').replace(/&amp;/g, '&').replace(/&lt;/g, '<').replace(/&gt;/g, '>').replace(/&quot;/g, '"').replace(/&#39;/g, "'").trim();

        return (
            <View style={styles.card}>
                <View style={styles.cardHeader}>
                    <Flag size={16} color={C.amber} />
                    <Text style={styles.signText} numberOfLines={1} ellipsizeMode="middle">
                        {item.sign.substring(0, 16)}...
                    </Text>
                </View>

                <Text style={styles.contentText} numberOfLines={2}>
                    {plainContent || "<Media Message>"}
                </Text>

                <View style={styles.statusRow}>
                    {isApproved ? (
                        <View style={styles.actionTakenContainer}>
                            <View style={styles.statusBadge}>
                                <CheckCircle size={14} color={C.green} />
                                <Text style={[styles.statusText, { color: C.green }]}>Action Taken</Text>
                            </View>
                            <View style={styles.modDivider} />
                            <View style={styles.modList}>
                                <Text style={styles.modListTitle}>Handled by:</Text>
                                <View style={styles.modAvatarsContainer}>
                                    {Array.isArray(item.mod_certs) && item.mod_certs.map((mod: any, index: number) => {
                                        if (!mod) return null;
                                        const modKey = typeof mod === 'string' ? mod : mod.mod_pub_key;
                                        if (!modKey) return null;
                                        // Generate a pseudo-random color based on the key
                                        const hue = modKey.length > 5 ? modKey.charCodeAt(5) * 10 % 360 : 0;
                                        return (
                                            <View key={`${item.sign}-mod-${index}`} style={[styles.modAvatar, { backgroundColor: `hsl(${hue}, 60%, 40%)`, zIndex: 10 - index }]}>
                                                <Text style={styles.modAvatarText}>{modKey.substring(0, 1).toUpperCase()}</Text>
                                            </View>
                                        );
                                    })}
                                    <Text style={styles.modCountText}>
                                        {Array.isArray(item.mod_certs) ? item.mod_certs.length : 0} Mod{Array.isArray(item.mod_certs) && item.mod_certs.length === 1 ? '' : 's'}
                                    </Text>
                                </View>
                            </View>
                        </View>
                    ) : (
                        <View style={styles.statusBadge}>
                            <Clock size={14} color={C.amber} />
                            {pendingStatus[item.sign] && pendingStatus[item.sign].total > 0 ? (
                                <Text style={[styles.statusText, { color: C.amber }]}>
                                    Approved by {pendingStatus[item.sign].approved}/{Math.floor(pendingStatus[item.sign].total / 2) + 1} Mods
                                </Text>
                            ) : (
                                <Text style={[styles.statusText, { color: C.amber }]}>Under Review</Text>
                            )}
                        </View>
                    )}
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
                <Text style={styles.title}>My Reports</Text>
                <View style={{ width: 40 }} />
            </View>

            {loading ? (
                <View style={styles.centerContainer}>
                    <Text style={styles.emptyText}>Loading...</Text>
                </View>
            ) : reportedMessages.length === 0 ? (
                <View style={styles.centerContainer}>
                    <Flag size={48} color={C.border} style={{ marginBottom: 16 }} />
                    <Text style={styles.emptyText}>You haven't reported any messages yet.</Text>
                </View>
            ) : (
                <FlatList
                    data={reportedMessages}
                    keyExtractor={(item) => item.sign}
                    renderItem={renderItem}
                    contentContainerStyle={styles.listContainer}
                />
            )}
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
        gap: 12,
    },
    card: {
        backgroundColor: C.card,
        borderRadius: 12,
        padding: 16,
        borderWidth: 1,
        borderColor: C.border,
    },
    cardHeader: {
        flexDirection: 'row',
        alignItems: 'center',
        gap: 8,
        marginBottom: 8,
    },
    signText: {
        color: C.muted,
        fontSize: 12,
        flex: 1,
    },
    contentText: {
        color: C.text,
        fontSize: 14,
        lineHeight: 20,
        marginBottom: 12,
    },
    statusRow: {
        flexDirection: 'row',
        justifyContent: 'flex-start',
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
    }
});
