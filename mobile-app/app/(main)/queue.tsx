import { View, Text, StyleSheet } from 'react-native';
import { Colors } from '@/constants/theme';

export default function QueueScreen() {
    return (
        <View style={styles.container}>
            <Text style={styles.text}>Queue Screen</Text>
        </View>
    );
}

const styles = StyleSheet.create({
    container: {
        flex: 1,
        backgroundColor: Colors.dark.background,
        alignItems: 'center',
        justifyContent: 'center',
    },
    text: {
        color: Colors.dark.text,
    },
});
