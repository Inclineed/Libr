import React, { useState } from 'react';
import { View, Text, TextInput, TouchableOpacity, StyleSheet, KeyboardAvoidingView, Platform, ScrollView, Alert } from 'react-native';
import { Stack, useRouter } from 'expo-router';
import { X, Image as ImageIcon, Send, XCircle } from 'lucide-react-native';
import LibrCore, { RetMsgCert, SendResult } from '@/modules/LibrCore';
import { Colors } from '@/constants/theme';
import { useAppStore } from '@/store/useAppStore';
import * as ImagePicker from 'expo-image-picker';
import * as ImageManipulator from 'expo-image-manipulator';
import { Image } from 'expo-image';

export default function CreateMessageModal() {
  const router = useRouter();
  const { state, addMessage, removeMessage } = useAppStore();
  const [title, setTitle] = useState('');
  const [content, setContent] = useState('');
  const [pendingImages, setPendingImages] = useState<string[]>([]); // base64 strings
  const [isProcessing, setIsProcessing] = useState(false);

  const handlePickImage = async () => {
    try {
      const result = await ImagePicker.launchImageLibraryAsync({
        mediaTypes: ['images'],
        allowsEditing: false,
        quality: 1,
      });

      if (!result.canceled && result.assets && result.assets.length > 0) {
        setIsProcessing(true);
        const { uri } = result.assets[0];

        // 1. Resize and compress like desktop (max 800px)
        const manipResult = await ImageManipulator.manipulateAsync(
          uri,
          [{ resize: { width: 800 } }],
          { compress: 0.8, format: ImageManipulator.SaveFormat.JPEG, base64: true }
        );

        if (manipResult.base64) {
          setPendingImages((prev: string[]) => [...prev, `data:image/jpeg;base64,${manipResult.base64}`]);
        }
      }
    } catch (e) {
      Alert.alert('Error', 'Failed to pick image');
      console.error(e);
    } finally {
      setIsProcessing(false);
    }
  };

  const removeImage = (index: number) => {
    setPendingImages((prev: string[]) => prev.filter((_: string, i: number) => i !== index));
  };

  const handleSend = async () => {
    const trimmedTitle = title.trim();
    const trimmedBody = content.trim();
    if (!trimmedBody && pendingImages.length === 0) return;

    const imagesHtml = pendingImages.map((src: string) => `<img src="${src}" />`).join('');
    const bodyWithImages = `<BODY>${trimmedBody}${imagesHtml}</BODY>`;

    const fullContent = trimmedTitle ? `<HEAD>${trimmedTitle}</HEAD>${bodyWithImages}` : bodyWithImages;

    // 1. Optimistic UI: Create a temporary certificate to show immediately
    const tempCert: RetMsgCert = {
      public_key: state.publicKey,
      sign: `temp-${Date.now()}-${Math.random().toString(36).substr(2, 9)}`,
      msg: {
        content: fullContent,
        ts: Math.floor(Date.now() / 1000),
      },
      mod_certs: [],
      deleted: '0'
    };

    // 2. Add message to feed local state
    addMessage(tempCert);

    // 3. Dismiss modal immediately
    if (router.canGoBack()) {
      router.back();
    } else {
      router.replace('/');
    }

    // 4. Background: Perform actual deliverya
    try {
      const raw: string = await LibrCore.sendTextMessage(fullContent);
      const result: SendResult = JSON.parse(raw);

      if (result.status === 'sent' || result.status?.startsWith('sent:') || result.status === 'pending_manual') {
        if (result.status === 'pending_manual') {
          Alert.alert('Manual Approval', 'Message contains an image and has been sent for manual approval.');
        }

        // 5. Replace temp with real data once confirmed
        removeMessage(tempCert.sign);
        const realCert: RetMsgCert = {
          public_key: state.publicKey,
          sign: result.sign,
          msg: {
            content: fullContent,
            ts: result.ts,
          },
          mod_certs: result.mod_certs || [],
          deleted: '0'
        };
        addMessage(realCert);
      } else {
        Alert.alert('Send Failed', result.status || 'Unknown error');
      }
    } catch (e) {
      console.error('[BackgroundSend]', e);
      // Optional: notify of failure or keep as "permanent optimistic error state"?
      // I'll just log for now so it doesn't crash.
    }
  };

  return (
    <KeyboardAvoidingView
      style={styles.container}
      behavior={Platform.OS === 'ios' ? 'padding' : undefined}
    >
      <Stack.Screen
        options={{
          headerShown: true,
          title: 'Spill Some Gossip',
          headerStyle: { backgroundColor: Colors.dark.background },
          headerTintColor: Colors.dark.text,
          headerLeft: () => (
            <TouchableOpacity
              onPress={() => router.canGoBack() ? router.back() : router.replace('/')}
              style={{ padding: 8, marginLeft: -8 }}
            >
              <X size={24} color={Colors.dark.icon} />
            </TouchableOpacity>
          )
        }}
      />

      <View style={styles.content}>
        <TextInput
          style={styles.inputTitle}
          placeholder="Title (optional)"
          placeholderTextColor={Colors.dark.icon}
          value={title}
          onChangeText={setTitle}
        />

        <View style={styles.divider} />

        <TextInput
          style={styles.inputBody}
          placeholder="What's the gossip?"
          placeholderTextColor={Colors.dark.icon}
          multiline
          textAlignVertical="top"
          value={content}
          onChangeText={setContent}
          autoFocus={!isProcessing}
        />

        {pendingImages.length > 0 && (
          <ScrollView horizontal showsHorizontalScrollIndicator={false} style={styles.imageSelector}>
            {pendingImages.map((src: string, index: number) => (
              <View key={index} style={styles.imageThumbContainer}>
                <Image source={src} style={styles.imageThumb} />
                <TouchableOpacity
                  style={styles.removeBtn}
                  onPress={() => removeImage(index)}
                >
                  <XCircle size={20} color={'#ef4444'} fill={Colors.dark.background} />
                </TouchableOpacity>
              </View>
            ))}
          </ScrollView>
        )}
      </View>

      <View style={styles.footer}>
        <TouchableOpacity
          style={styles.attachBtn}
          onPress={handlePickImage}
          disabled={isProcessing}
        >
          {isProcessing ? (
            <View style={{ width: 24, height: 24, borderRadius: 12, borderTopColor: Colors.dark.tint, borderWidth: 2 }} />
          ) : (
            <ImageIcon size={24} color={Colors.dark.icon} />
          )}
        </TouchableOpacity>

        <TouchableOpacity
          style={[styles.sendBtn, (!content.trim() && pendingImages.length === 0) && styles.sendBtnDisabled]}
          disabled={!content.trim() && pendingImages.length === 0}
          onPress={handleSend}
        >
          <Send size={20} color="#fff" />
        </TouchableOpacity>
      </View>
    </KeyboardAvoidingView>
  );
}

const styles = StyleSheet.create({
  container: {
    flex: 1,
    backgroundColor: Colors.dark.background,
  },
  content: {
    flex: 1,
    padding: 16,
  },
  inputTitle: {
    fontSize: 20,
    fontWeight: '600',
    color: Colors.dark.text,
    paddingVertical: 12,
  },
  divider: {
    height: 1,
    backgroundColor: Colors.dark.border,
    marginBottom: 12,
  },
  inputBody: {
    flex: 1,
    fontSize: 16,
    color: Colors.dark.text,
    lineHeight: 24,
  },
  footer: {
    flexDirection: 'row',
    alignItems: 'center',
    justifyContent: 'space-between',
    paddingHorizontal: 20,
    paddingVertical: 12,
    paddingBottom: Platform.OS === 'ios' ? 34 : 12,
    borderTopWidth: 1,
    borderTopColor: Colors.dark.border,
    backgroundColor: Colors.dark.background,
  },
  attachBtn: {
    padding: 8,
  },
  sendBtn: {
    backgroundColor: Colors.dark.tint,
    width: 48,
    height: 48,
    borderRadius: 24,
    alignItems: 'center',
    justifyContent: 'center',
    shadowColor: Colors.dark.tint,
    shadowOffset: { width: 0, height: 4 },
    shadowOpacity: 0.3,
    shadowRadius: 6,
    elevation: 4,
  },
  sendBtnDisabled: {
    opacity: 0.4,
    shadowOpacity: 0,
  },
  imageSelector: {
    maxHeight: 120,
    marginTop: 10,
  },
  imageThumbContainer: {
    marginRight: 10,
    position: 'relative',
    paddingVertical: 10,
  },
  imageThumb: {
    width: 100,
    height: 100,
    borderRadius: 12,
    borderWidth: 1,
    borderColor: Colors.dark.border,
  },
  removeBtn: {
    position: 'absolute',
    top: 0,
    right: -5,
    zIndex: 1,
  }
});
