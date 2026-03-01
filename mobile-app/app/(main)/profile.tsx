import React, { useEffect, useState } from 'react';
import { View, Text, StyleSheet, TouchableOpacity, Platform } from 'react-native';
import { Image } from 'expo-image';
import { SvgXml } from 'react-native-svg';
import { Colors } from '@/constants/theme';
import { useAppStore } from '@/store/useAppStore';
import LibrCore from '@/modules/LibrCore';
import { Menu, X, Shield, RefreshCcw, ArrowLeft } from 'lucide-react-native';
import { useSidebar } from './_layout';
import { useRouter } from 'expo-router';

function getAvatarUri(svg: string | null): string | null {
    if (!svg) return null;
    if (svg.startsWith('<svg')) {
        return `data:image/svg+xml;utf8,${encodeURIComponent(svg)}`;
    }
    if (!svg.startsWith('data:')) {
        return `data:image/svg+xml;base64,${svg}`;
    }
    return svg;
}

export default function ProfileScreen() {
    const { state } = useAppStore();
    const router = useRouter();
    const { toggleSidebar } = useSidebar();

    // Avatar and Alias generation
    const [alias, setAlias] = useState('…');
    const [avatarSvg, setAvatarSvg] = useState<string | null>(null);

    // Determine moderator status
    const [isModerator, setIsModerator] = useState(false);

    useEffect(() => {
        console.log('[Profile] useEffect trigger. publicKey:', state.publicKey ? (state.publicKey.slice(0, 10) + '...') : 'EMPTY');
        if (state.publicKey) {
            (async () => {
                try {
                    console.log('[Profile] Generating alias...');
                    const a = await LibrCore.generateAlias(state.publicKey);
                    console.log('[Profile] Alias result:', a);
                    setAlias(a);

                    console.log('[Profile] Generating avatar...');
                    const av = await LibrCore.generateAvatar(state.publicKey);
                    console.log('[Profile] Avatar result length:', av?.length);
                    setAvatarSvg(av);

                    const mod = await LibrCore.amIMod();
                    setIsModerator(mod);
                } catch (err: any) {
                    console.error('[Profile] Error in identity generation:', err);
                }
            })();
        } else {
            console.warn('[Profile] No publicKey available in store.');
        }
    }, [state.publicKey]);

    return (
        <View style={styles.container}>
            {/* Header */}
            <View style={styles.header}>
                <TouchableOpacity onPress={() => router.back()} style={styles.backButton}>
                    <ArrowLeft size={24} color={Colors.dark.text} />
                </TouchableOpacity>
                <Text style={styles.headerTitle}>Profile</Text>
                <View style={styles.statusContainer}>
                    <View style={[styles.statusDot, { backgroundColor: state.connectionStatus === 'connected' ? '#22c55e' : '#eab308' }]} />
                    <Text style={[styles.statusText, { color: state.connectionStatus === 'connected' ? '#22c55e' : '#eab308' }]}>
                        {state.connectionStatus === 'connected' ? 'Connected' : 'Connecting...'}
                    </Text>
                </View>
            </View>

            <View style={styles.content}>
                <View style={styles.card}>
                    {/* Avatar */}
                    <View style={styles.avatarContainer}>
                        {avatarSvg ? (
                            <Image
                                source={getAvatarUri(avatarSvg)}
                                style={styles.avatar}
                                contentFit="cover"
                            />
                        ) : (
                            <View style={styles.avatarFallback} />
                        )}
                    </View>

                    {/* Alias */}
                    <Text style={styles.aliasText}>{alias}</Text>

                    {/* Badge */}
                    {isModerator && (
                        <View style={styles.badge}>
                            <Shield size={14} color={Colors.dark.background} fill={Colors.dark.background} />
                            <Text style={styles.badgeText}>Moderator</Text>
                        </View>
                    )}

                    <View style={styles.spacer} />

                    {/* Actions */}
                    <TouchableOpacity style={styles.resetBtn}>
                        <RefreshCcw size={20} color={Colors.dark.icon} />
                        <Text style={styles.resetBtnText}>Reset Identity</Text>
                    </TouchableOpacity>

                    <TouchableOpacity
                        style={styles.closeBtn}
                        onPress={() => router.canGoBack() ? router.back() : router.replace('/')}
                    >
                        <Text style={styles.closeBtnText}>Close</Text>
                    </TouchableOpacity>
                </View>
            </View>
        </View>
    );
}

const styles = StyleSheet.create({
    container: {
        flex: 1,
        backgroundColor: Colors.dark.background,
    },
    header: {
        flexDirection: 'row',
        alignItems: 'center',
        justifyContent: 'space-between',
        paddingHorizontal: 16,
        paddingTop: Platform.OS === 'ios' ? 56 : 40,
        paddingBottom: 16,
        borderBottomWidth: 1,
        borderBottomColor: Colors.dark.border,
    },
    headerBtn: {
        padding: 8,
    },
    menuBtn: {
        backgroundColor: Colors.dark.border,
        borderRadius: 8,
        padding: 6,
    },
    headerTitle: {
        fontSize: 20,
        fontWeight: '700',
        color: Colors.dark.text,
    },
    backButton: {
        padding: 8,
    },
    statusContainer: {
        flexDirection: 'row',
        alignItems: 'center',
        gap: 6,
    },
    statusDot: {
        width: 8,
        height: 8,
        borderRadius: 4,
    },
    statusText: {
        fontSize: 12,
        fontWeight: '600',
    },
    content: {
        flex: 1,
        padding: 24,
        alignItems: 'center',
        justifyContent: 'center',
    },
    card: {
        width: '100%',
        backgroundColor: Colors.dark.primary,
        borderRadius: 24,
        padding: 32,
        alignItems: 'center',
        borderWidth: 1,
        borderColor: Colors.dark.border,
        shadowColor: '#000',
        shadowOffset: { width: 0, height: 8 },
        shadowOpacity: 0.3,
        shadowRadius: 12,
        elevation: 8,
    },
    avatarContainer: {
        width: 120,
        height: 120,
        borderRadius: 60,
        backgroundColor: Colors.dark.border,
        overflow: 'hidden',
        marginBottom: 24,
        borderWidth: 4,
        borderColor: Colors.dark.background,
    },
    avatar: {
        width: '100%',
        height: '100%',
    },
    avatarFallback: {
        flex: 1,
        backgroundColor: Colors.dark.icon,
    },
    aliasText: {
        fontSize: 24,
        fontWeight: '800',
        color: Colors.dark.text,
        marginBottom: 12,
        textAlign: 'center',
    },
    badge: {
        flexDirection: 'row',
        alignItems: 'center',
        backgroundColor: '#22c55e', // Vibrant green
        paddingHorizontal: 12,
        paddingVertical: 6,
        borderRadius: 16,
        gap: 6,
    },
    badgeText: {
        color: Colors.dark.background,
        fontWeight: '800',
        fontSize: 13,
        textTransform: 'uppercase',
    },
    spacer: {
        height: 32,
    },
    resetBtn: {
        flexDirection: 'row',
        alignItems: 'center',
        backgroundColor: Colors.dark.background,
        paddingHorizontal: 20,
        paddingVertical: 14,
        borderRadius: 12,
        borderWidth: 1,
        borderColor: Colors.dark.border,
        width: '100%',
        justifyContent: 'center',
        marginBottom: 12,
        gap: 8,
    },
    resetBtnText: {
        color: Colors.dark.icon,
        fontSize: 16,
        fontWeight: '600',
    },
    closeBtn: {
        backgroundColor: Colors.dark.tint,
        paddingHorizontal: 20,
        paddingVertical: 14,
        borderRadius: 12,
        width: '100%',
        alignItems: 'center',
    },
    closeBtnText: {
        color: '#fff',
        fontSize: 16,
        fontWeight: '700',
    }
});
