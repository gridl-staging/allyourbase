# allyourbase (Dart SDK)

Pure Dart SDK for the Allyourbase API.

The package mirrors `@allyourbase/js` and supports:

- Auth (register, login, refresh, logout, OAuth redirect flow, password reset, email verification)
- Records CRUD + batch
- Realtime SSE subscriptions with reconnect
- Storage upload/list/delete/signed URLs
- RPC calls
- API key auth
- Push device token registration

## Install

```yaml
dependencies:
  allyourbase: ^0.1.0
```

## Quick start

```dart
import 'package:allyourbase/allyourbase.dart';

Future<void> main() async {
  final client = AYBClient('http://localhost:8090');

  await client.auth.login('user@example.com', 'password');

  final created = await client.records.create('posts', {
    'title': 'Hello from Dart',
    'published': true,
  });

  final posts = await client.records.list(
    'posts',
    params: const ListParams(sort: '-created_at', perPage: 10),
  );

  print('created id: ${created['id']}');
  print('total posts: ${posts.totalItems}');

  client.close();
}
```

## Initialize the client

```dart
final client = AYBClient('https://api.example.com');
```

With custom HTTP transport:

```dart
import 'package:http/http.dart' as http;

final client = AYBClient(
  'https://api.example.com',
  httpClient: http.Client(),
);
```

## Auth

```dart
await client.auth.register('new@example.com', 'strong-password');
await client.auth.login('user@example.com', 'password');

final me = await client.auth.me();
print(me.email);

await client.auth.refresh();
await client.auth.resendVerification();
await client.auth.requestPasswordReset('user@example.com');
await client.auth.confirmPasswordReset('token-from-email', 'new-password');
await client.auth.verifyEmail('verification-token');

await client.auth.logout();
```

### Token management

```dart
client.setTokens('access-token', 'refresh-token');
client.clearTokens();

client.setApiKey('ayb_api_key_xxx');
client.clearApiKey();
```

### Auth state listener

```dart
final cancel = client.onAuthStateChange((event, session) {
  print('auth event: $event');
  print('session token exists: ${session?.token != null}');
});

cancel();
```

## Records

```dart
final list = await client.records.list(
  'todos',
  params: const ListParams(
    page: 1,
    perPage: 20,
    filter: "completed=false",
    sort: '-created_at',
    expand: 'owner',
  ),
);

final record = await client.records.get('todos', '123');

await client.records.create('todos', {'title': 'Ship v1'});
await client.records.update('todos', '123', {'completed': true});
await client.records.delete('todos', '123');

final batch = await client.records.batch('todos', const [
  BatchOperation(method: 'create', body: {'title': 'A'}),
  BatchOperation(method: 'create', body: {'title': 'B'}),
]);
print(batch.length);
```

### GeoJSON columns (PostGIS)

Geometry/geography columns are plain JSON maps — no special types needed:

```dart
final place = await client.records.create('places', {
  'name': 'Central Park',
  'location': {'type': 'Point', 'coordinates': [-73.9654, 40.7829]},
});
final loc = place['location'] as Map<String, Object?>; // GeoJSON object
```

## Realtime

```dart
final unsubscribe = client.realtime.subscribe(['todos', 'comments'], (event) {
  print('${event.action} on ${event.table}: ${event.record}');
});

unsubscribe();
```

## Storage

```dart
import 'dart:typed_data';

final bytes = Uint8List.fromList([104, 101, 108, 108, 111]);
final uploaded = await client.storage.upload(
  'avatars',
  bytes,
  'hello.txt',
  contentType: 'text/plain',
);

final directUrl = client.storage.downloadUrl('avatars', uploaded.name);
final signedUrl = await client.storage.getSignedUrl('avatars', uploaded.name);

final files = await client.storage.list('avatars', prefix: 'user_');
await client.storage.delete('avatars', uploaded.name);
```

## OAuth (redirect flow)

```dart
final auth = await client.auth.signInWithOAuth(
  'google',
  urlCallback: (url) async {
    // In Flutter, launch this URL with url_launcher.
  },
);

print(auth.token);
```

Handle deep-link callback:

```dart
final response = client.auth.handleOAuthRedirect(callbackUri);
if (response != null) {
  print('Signed in via OAuth redirect');
}
```

## Push device tokens

```dart
await client.push.registerDevice(
  appId: '00000000-0000-0000-0000-000000000001',
  provider: 'fcm',
  platform: 'android',
  token: '<device-token>',
  deviceName: 'Pixel 8',
);

final devices = await client.push.listDevices('00000000-0000-0000-0000-000000000001');
await client.push.revokeDevice(devices.first.id);
```

## RPC

```dart
final result = await client.rpc<Map<String, Object?>>(
  'leaderboard_totals',
  args: {'club_id': 'abc123'},
);
print(result);
```

## Async token persistence

```dart
import 'package:flutter_secure_storage/flutter_secure_storage.dart';

final secureStorage = FlutterSecureStorage();
final initial = await secureStorage.read(key: 'ayb_auth');

final store = AsyncAuthStore(
  save: (data) => secureStorage.write(key: 'ayb_auth', value: data),
  clear: () => secureStorage.delete(key: 'ayb_auth'),
  initial: initial,
);

final client = AYBClient('https://api.example.com');
if (store.token != null && store.refreshToken != null) {
  client.setTokens(store.token!, store.refreshToken!);
}

client.onAuthStateChange((event, session) {
  if (session == null) {
    store.clear();
  } else {
    store.save(session.token, session.refreshToken);
  }
});
```

## Errors

```dart
try {
  await client.records.get('posts', 'missing-id');
} on AYBError catch (error) {
  print(error.status);
  print(error.code);
  print(error.message);
}
```

## Example app

A runnable console example is available at `example/main.dart`.

```bash
cd sdk_dart
dart run example/main.dart
```

## License

MIT
