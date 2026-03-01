import React, { useState } from 'react';
import { View, Text, TextInput, TouchableOpacity, StyleSheet, KeyboardAvoidingView, Platform } from 'react-native';
import { Stack, useRouter } from 'expo-router';
import { X, Image as ImageIcon, Send } from 'lucide-react-native';
import LibrCore from '@/modules/LibrCore';
import { Colors } from '@/constants/theme';

export default function CreateMessageModal() {
  const router = useRouter();
  const [title, setTitle] = useState('');
  const [content, setContent] = useState('');

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
          autoFocus
        />
      </View>

      <View style={styles.footer}>
        <TouchableOpacity style={styles.attachBtn}>
          <ImageIcon size={24} color={Colors.dark.icon} />
        </TouchableOpacity>

        <TouchableOpacity
          style={[styles.sendBtn, !content.trim() && styles.sendBtnDisabled]}
          disabled={!content.trim()}
          onPress={async () => {
            try {
              const body = title.trim() ? `<HEAD>${title.trim()}</HEAD><BODY>${content.trim()}</BODY>` : content.trim();
              await LibrCore.sendTextMessage(body);
              if (router.canGoBack()) {
                router.back();
              } else {
                router.replace('/');
              }
            } catch (e) {
              console.error(e);
            }
          }}
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
  }
});
