<!-- audited 2026-03-21 -->
# Flutter SDK

Use the `allyourbase` package from Flutter to access auth, records, realtime SSE, storage, RPC, and push-device APIs.

The SDK itself is pure Dart (no Flutter dependency), so it also works in server-side Dart and CLI apps.

## Install

Add the SDK package:

```yaml
dependencies:
  allyourbase: ^0.1.0
```

Optional packages used in examples below:

```yaml
dependencies:
  flutter_secure_storage: ^9.2.2
  url_launcher: ^6.3.1
  firebase_messaging: ^15.1.6
  image_picker: ^1.1.2
```

## Initialize

`AYBClient` constructor:

- `AYBClient(String baseUrl, {http.Client? httpClient, RealtimeOptions realtimeOptions = const RealtimeOptions()})`

```dart
import 'package:allyourbase/allyourbase.dart';
import 'package:http/http.dart' as http;

final ayb = AYBClient(
  'http://10.0.2.2:8090',
  httpClient: http.Client(),
  realtimeOptions: const RealtimeOptions(maxReconnectAttempts: 5),
);
```

Use `10.0.2.2` for Android emulator localhost, and `localhost` for iOS simulator.

`AYBClient` exposes sub-clients: `auth`, `records`, `storage`, `realtime`, `push`, plus `rpc()`.

## Exports

`package:allyourbase/allyourbase.dart` exports:

- `client.dart` (`AYBClient`, `AuthClient`, `RecordsClient`, `StorageClient`, `RealtimeClient`, `PushClient`, `RealtimeOptions`)
- `types.dart` (`ListParams`, `GetParams`, `AuthResponse`, `User`, `ListResponse`, `RealtimeEvent`, `StorageObject`, `BatchOperation`, `BatchResult`, `DeviceToken`)
- `errors.dart` (`AYBError`)
- `auth_store.dart` (`AuthStore`, `AsyncAuthStore`, `AuthStoreEvent`)

## Auth with persistent tokens

Use `AsyncAuthStore` with `flutter_secure_storage`:

```dart
import 'package:allyourbase/allyourbase.dart';
import 'package:flutter_secure_storage/flutter_secure_storage.dart';

final secureStorage = FlutterSecureStorage();
final initial = await secureStorage.read(key: 'ayb_auth');

final authStore = AsyncAuthStore(
  save: (data) => secureStorage.write(key: 'ayb_auth', value: data),
  clear: () => secureStorage.delete(key: 'ayb_auth'),
  initial: initial,
);

final ayb = AYBClient('https://api.example.com');
if (authStore.token != null && authStore.refreshToken != null) {
  ayb.setTokens(authStore.token!, authStore.refreshToken!);
}

ayb.onAuthStateChange((event, session) {
  if (session == null) {
    authStore.clear();
  } else {
    authStore.save(session.token, session.refreshToken);
  }
});
```

Login/register:

```dart
await ayb.auth.register('new@example.com', 'password');
await ayb.auth.login('user@example.com', 'password');
final me = await ayb.auth.me();
```

Also available on `ayb.auth`:

- `refresh()`
- `logout()`
- `deleteAccount()`
- `requestPasswordReset(String email)`
- `confirmPasswordReset(String token, String password)`
- `verifyEmail(String token)`
- `resendVerification()`
- `signInWithOAuth(...)`
- `handleOAuthRedirect(Uri uri)`

## Records

```dart
final created = await ayb.records.create('workouts', {
  'name': 'Tempo Run',
  'distance_km': 8.2,
  'completed': true,
});

final record = await ayb.records.get('workouts', created['id'].toString());

await ayb.records.update('workouts', created['id'].toString(), {
  'name': 'Tempo Run (updated)',
});

final list = await ayb.records.list(
  'workouts',
  params: const ListParams(
    filter: 'completed=true',
    sort: '-created_at',
    perPage: 20,
  ),
);

await ayb.records.delete('workouts', created['id'].toString());
```

### Batch

```dart
final results = await ayb.records.batch('workouts', [
  const BatchOperation(method: 'create', body: {'name': 'Easy Run'}),
  const BatchOperation(method: 'delete', id: '42'),
]);
```

## Realtime (SSE)

```dart
final unsubscribe = ayb.realtime.subscribe(['workouts'], (event) {
  // event.action: create | update | delete
  // event.table: workouts
  // event.record: map payload
  print('Realtime: ${event.action} ${event.record}');
});

// Call on screen dispose
unsubscribe();
```

## Storage upload from camera/gallery

Pick an image with `image_picker`, convert to `Uint8List`, upload:

```dart
import 'dart:typed_data';
import 'package:image_picker/image_picker.dart';

final picker = ImagePicker();
final photo = await picker.pickImage(source: ImageSource.gallery);
if (photo != null) {
  final bytes = await photo.readAsBytes(); // Uint8List

  final uploaded = await ayb.storage.upload(
    'profile-photos',
    bytes,
    photo.name,
    contentType: 'image/jpeg',
  );

  final signedUrl = await ayb.storage.getSignedUrl('profile-photos', uploaded.name);
  final downloadUrl = ayb.storage.downloadUrl('profile-photos', uploaded.name);
  final files = await ayb.storage.list('profile-photos', prefix: 'user_', limit: 20);
  await ayb.storage.delete('profile-photos', uploaded.name);
  print(signedUrl);
  print(downloadUrl);
  print(files.totalItems);
}
```

## OAuth redirect flow with deep links

1. Start OAuth using `signInWithOAuth` and launch URL with `url_launcher`.
2. Handle the deep-link callback URI in your app.
3. Pass callback URI to `handleOAuthRedirect`.

```dart
import 'package:url_launcher/url_launcher.dart';

await ayb.auth.signInWithOAuth(
  'google',
  scopes: ['openid', 'email', 'profile'],
  urlCallback: (url) async {
    final launched = await launchUrl(Uri.parse(url), mode: LaunchMode.externalApplication);
    if (!launched) {
      throw Exception('Could not launch OAuth URL');
    }
  },
);
```

In your deep-link handler:

```dart
final auth = ayb.auth.handleOAuthRedirect(callbackUri);
if (auth != null) {
  // token + refresh token stored on client
  // optionally fetch full profile:
  await ayb.auth.me();
}
```

## Push device token registration (FCM/APNS)

Register current FCM token for the signed-in user:

```dart
import 'package:firebase_messaging/firebase_messaging.dart';

final fcmToken = await FirebaseMessaging.instance.getToken();
if (fcmToken != null) {
  await ayb.push.registerDevice(
    appId: '00000000-0000-0000-0000-000000000001',
    provider: 'fcm',
    platform: 'android',
    token: fcmToken,
    deviceName: 'Pixel 8 Pro',
  );
}

final devices = await ayb.push.listDevices('00000000-0000-0000-0000-000000000001');
```

Revoke token on logout/device unlink:

```dart
await ayb.push.revokeDevice(deviceId);
```

## RPC and API keys

```dart
ayb.setApiKey('ayb_api_key_xxx');

final result = await ayb.rpc<Map<String, Object?>>(
  'leaderboard_totals',
  args: {'club_id': 'abc123'},
);

ayb.clearApiKey();
```

## GeoJSON columns (PostGIS)

When your database has PostGIS geometry/geography columns, the SDK handles GeoJSON values as plain Dart maps — no special types needed:

```dart
// Create a record with a GeoJSON Point
final place = await ayb.records.create('places', {
  'name': 'Central Park',
  'location': {
    'type': 'Point',
    'coordinates': [-73.9654, 40.7829],
  },
});

// Read it back — location is a Map<String, Object?>
final loc = place['location'] as Map<String, Object?>;
print(loc['type']);        // "Point"
print(loc['coordinates']); // [-73.9654, 40.7829]

// Update with a new location
await ayb.records.update('places', place['id'].toString(), {
  'location': {
    'type': 'Point',
    'coordinates': [-73.9700, 40.7850],
  },
});
```

Null geometry columns return `null`. Realtime SSE events also include GeoJSON in the record payload.

See the [PostGIS guide](/guide/postgis) for spatial query patterns using RPC functions.

## Errors

`AYBError` fields: `status`, `message`, `code`, `data`, `docUrl`.

```dart
try {
  await ayb.records.get('workouts', 'missing-id');
} on AYBError catch (error) {
  print(error.status);
  print(error.code);
  print(error.message);
  print(error.data);
  print(error.docUrl);
}
```
