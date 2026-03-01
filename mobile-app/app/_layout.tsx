import { DarkTheme, ThemeProvider } from '@react-navigation/native';
import { Stack } from 'expo-router';
import { StatusBar } from 'expo-status-bar';
import 'react-native-reanimated';
import React from 'react';
import { Text, View, ScrollView } from 'react-native';

import { Colors } from '@/constants/theme';
import { AppStoreProvider } from '@/store/useAppStore';

export const unstable_settings = {
  anchor: '(main)',
};

const DarkNavyTheme = {
  ...DarkTheme,
  colors: {
    ...DarkTheme.colors,
    background: Colors.dark.background,
    card: Colors.dark.primary,
    text: Colors.dark.text,
    border: Colors.dark.border,
    primary: Colors.dark.tint,
  },
};

class ErrorBoundary extends React.Component<
  { children: React.ReactNode },
  { error: Error | null }
> {
  constructor(props: { children: React.ReactNode }) {
    super(props);
    this.state = { error: null };
  }
  static getDerivedStateFromError(error: Error) {
    return { error };
  }
  render() {
    if (this.state.error) {
      return (
        <ScrollView style={{ flex: 1, padding: 24, paddingTop: 60, backgroundColor: Colors.dark.background }}>
          <Text style={{ color: '#ef4444', fontSize: 18, fontWeight: 'bold', marginBottom: 12 }}>
            App Error
          </Text>
          <Text style={{ color: Colors.dark.text, fontSize: 13, fontFamily: 'monospace' }}>
            {this.state.error.message}\n\n{this.state.error.stack}
          </Text>
        </ScrollView>
      );
    }
    return this.props.children;
  }
}

export default function RootLayout() {
  return (
    <ErrorBoundary>
      <AppStoreProvider>
        <ThemeProvider value={DarkNavyTheme}>
          <Stack>
            <Stack.Screen name="(main)" options={{ headerShown: false }} />
            <Stack.Screen name="modal" options={{ presentation: 'modal', title: 'Modal' }} />
          </Stack>
          <StatusBar style="light" />
        </ThemeProvider>
      </AppStoreProvider>
    </ErrorBoundary>
  );
}
