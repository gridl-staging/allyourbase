# Dart SDK example

This example demonstrates:

- login
- records CRUD
- realtime subscribe/unsubscribe
- storage upload/downloadUrl/signedUrl/delete

## Requirements

- Running AYB server
- Existing user credentials
- Existing storage bucket (default: `sdk-dart-demo`)

## Run

```bash
cd sdk_dart
AYB_URL=http://localhost:8090 \
AYB_EMAIL=user@example.com \
AYB_PASSWORD=secret \
AYB_STORAGE_BUCKET=sdk-dart-demo \
dart run example/main.dart
```
