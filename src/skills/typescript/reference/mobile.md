# React Native Mobile Development

Cross-platform mobile applications with React Native and TypeScript.

## Stack Overview

| Component | Default | Purpose |
|-----------|---------|---------|
| Framework | React Native | Cross-platform |
| Development | Expo | Managed workflow |
| Navigation | React Navigation | Screen navigation |
| State | Zustand | Lightweight state |

## Expo vs Bare Workflow

### Use Expo for
- Standard app features
- Rapid prototyping
- Over-the-air updates
- Managed build process

### Use Bare for
- Custom native modules
- Advanced native features
- Third-party native dependencies

## Project Setup

```typescript
// app.json (Expo)
{
  "expo": {
    "name": "My App",
    "slug": "my-app",
    "version": "1.0.0",
    "ios": {
      "supportsTablet": true,
      "bundleIdentifier": "com.myapp"
    },
    "android": {
      "package": "com.myapp"
    }
  }
}
```

## Navigation

```typescript
// Type-safe navigation
import { createNativeStackNavigator, NativeStackScreenProps } from "@react-navigation/native-stack";

type RootStackParamList = {
  Home: undefined;
  Profile: { userId: string };
  Settings: undefined;
};

const Stack = createNativeStackNavigator<RootStackParamList>();

function App() {
  return (
    <NavigationContainer>
      <Stack.Navigator>
        <Stack.Screen name="Home" component={HomeScreen} />
        <Stack.Screen name="Profile" component={ProfileScreen} />
      </Stack.Navigator>
    </NavigationContainer>
  );
}

// Type-safe screen props
type ProfileProps = NativeStackScreenProps<RootStackParamList, "Profile">;

function ProfileScreen({ route, navigation }: ProfileProps) {
  const { userId } = route.params; // Type-safe!
  return (
    <View>
      <Text>User: {userId}</Text>
      <Button title="Settings" onPress={() => navigation.navigate("Settings")} />
    </View>
  );
}
```

## Platform-Specific Code

```typescript
import { Platform, StyleSheet } from "react-native";

const styles = StyleSheet.create({
  container: {
    paddingTop: Platform.OS === "ios" ? 20 : 0,
    ...Platform.select({
      ios: {
        shadowColor: "#000",
        shadowOffset: { width: 0, height: 2 },
        shadowOpacity: 0.3,
      },
      android: {
        elevation: 5,
      },
    }),
  },
});

// Platform-specific files
// Button.ios.tsx
export function Button() {
  return <IOSButton />;
}

// Button.android.tsx
export function Button() {
  return <AndroidButton />;
}

// Import picks correct file automatically
import { Button } from "./Button";
```

## Styling

```typescript
import { StyleSheet, Dimensions } from "react-native";

const { width, height } = Dimensions.get("window");

const styles = StyleSheet.create({
  container: {
    flex: 1,
    backgroundColor: "#fff",
    padding: 16,
  },
  card: {
    backgroundColor: "#f5f5f5",
    borderRadius: 8,
    padding: 16,
    shadowColor: "#000",
    shadowOffset: { width: 0, height: 2 },
    shadowOpacity: 0.1,
    elevation: 3,
  },
  image: {
    width: width - 32,
    height: 200,
    borderRadius: 8,
  },
});
```

## Native APIs

```typescript
// Camera
import { Camera } from "expo-camera";

function CameraScreen() {
  const [permission, requestPermission] = Camera.useCameraPermissions();

  if (!permission?.granted) {
    return <Button title="Grant Permission" onPress={requestPermission} />;
  }

  return <Camera style={{ flex: 1 }} />;
}

// Location
import * as Location from "expo-location";

async function getCurrentLocation() {
  const { status } = await Location.requestForegroundPermissionsAsync();
  if (status !== "granted") throw new Error("Permission denied");

  const location = await Location.getCurrentPositionAsync({});
  return {
    latitude: location.coords.latitude,
    longitude: location.coords.longitude,
  };
}

// AsyncStorage
import AsyncStorage from "@react-native-async-storage/async-storage";

const storage = {
  async get<T>(key: string): Promise<T | null> {
    const item = await AsyncStorage.getItem(key);
    return item ? JSON.parse(item) : null;
  },
  async set<T>(key: string, value: T) {
    await AsyncStorage.setItem(key, JSON.stringify(value));
  },
};
```

## Performance

```typescript
import { FlatList, memo, useCallback } from "react";

// Use FlatList for long lists
function ItemList({ items }: { items: Item[] }) {
  return (
    <FlatList
      data={items}
      keyExtractor={(item) => item.id}
      renderItem={({ item }) => <ItemCard item={item} />}
      removeClippedSubviews
      maxToRenderPerBatch={10}
      windowSize={5}
      getItemLayout={(_, index) => ({
        length: 100,
        offset: 100 * index,
        index,
      })}
    />
  );
}

// Memoize list items
const ItemCard = memo(({ item }: { item: Item }) => (
  <View style={styles.card}>
    <Text>{item.title}</Text>
  </View>
));

// Optimized images
import { Image } from "expo-image";

<Image
  source={{ uri }}
  style={{ width: 300, height: 200 }}
  contentFit="cover"
  placeholder="L6Pj0^jE..."
  transition={200}
/>
```

## Testing

```typescript
import { render, screen, fireEvent } from "@testing-library/react-native";

describe("LoginScreen", () => {
  it("submits form", () => {
    const onSubmit = jest.fn();
    render(<LoginScreen onSubmit={onSubmit} />);

    fireEvent.changeText(screen.getByPlaceholderText("Email"), "test@example.com");
    fireEvent.changeText(screen.getByPlaceholderText("Password"), "password");
    fireEvent.press(screen.getByText("Login"));

    expect(onSubmit).toHaveBeenCalledWith({
      email: "test@example.com",
      password: "password",
    });
  });
});
```

## Critical Rules

### Always
- Type navigation params
- Request permissions before use
- Test on both iOS and Android
- Use FlatList for long lists
- Handle safe areas

### Never
- Use web-only APIs
- Forget platform differences
- Skip permission checks
- Use ScrollView for long lists
- Ignore memory warnings
