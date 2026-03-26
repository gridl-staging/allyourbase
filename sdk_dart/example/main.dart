import 'dart:convert';
import 'dart:io';
import 'dart:typed_data';

import 'package:allyourbase/allyourbase.dart';

Future<void> main() async {
  final baseUrl = Platform.environment['AYB_URL'] ?? 'http://localhost:8090';
  final email = Platform.environment['AYB_EMAIL'];
  final password = Platform.environment['AYB_PASSWORD'];
  final bucket = Platform.environment['AYB_STORAGE_BUCKET'] ?? 'sdk-dart-demo';

  if (email == null || password == null) {
    stderr.writeln('Missing required env vars: AYB_EMAIL and AYB_PASSWORD');
    stderr.writeln('Optional: AYB_URL (default http://localhost:8090)');
    stderr.writeln('Optional: AYB_STORAGE_BUCKET (default sdk-dart-demo)');
    exitCode = 64;
    return;
  }

  final client = AYBClient(baseUrl);
  void Function()? unsubscribe;

  try {
    print('Logging in to $baseUrl ...');
    await client.auth.login(email, password);
    print('Authenticated as ${email.toLowerCase()}');

    final cancelAuthListener = client.onAuthStateChange((event, session) {
      print('Auth event: $event (has session: ${session != null})');
    });

    unsubscribe = client.realtime.subscribe(['todos'], (event) {
      print('Realtime event: ${event.action} ${event.table} ${event.record}');
    });

    final created = await client.records.create('todos', {
      'title': 'Dart SDK demo ${DateTime.now().toUtc().toIso8601String()}',
      'completed': false,
    });
    final createdId = created['id'].toString();
    print('Created todo id=$createdId');

    final loaded = await client.records.get('todos', createdId);
    print('Fetched todo title=${loaded['title']}');

    await client.records.update('todos', createdId, {'completed': true});
    print('Updated todo id=$createdId completed=true');

    final list = await client.records.list(
      'todos',
      params: const ListParams(sort: '-created_at', perPage: 5),
    );
    print('List count (first page): ${list.items.length}');

    final batch = await client.records.batch('todos', [
      const BatchOperation(method: 'create', body: {'title': 'Batch A'}),
      const BatchOperation(method: 'create', body: {'title': 'Batch B'}),
    ]);
    print('Batch operations completed: ${batch.length}');

    final upload = await client.storage.upload(
      bucket,
      Uint8List.fromList(utf8.encode('hello from dart sdk example')),
      'sdk_dart_example.txt',
      contentType: 'text/plain',
    );
    print('Uploaded storage object: ${upload.name}');

    final downloadUrl = client.storage.downloadUrl(bucket, upload.name);
    final signedUrl = await client.storage.getSignedUrl(bucket, upload.name);
    print('Download URL: $downloadUrl');
    print('Signed URL:   $signedUrl');

    await client.storage.delete(bucket, upload.name);
    print('Deleted uploaded object: ${upload.name}');

    await client.records.delete('todos', createdId);
    print('Deleted todo id=$createdId');

    cancelAuthListener();
    print('Done');
  } on AYBError catch (error) {
    stderr.writeln(
        'AYBError ${error.status} (${error.code ?? 'no-code'}): ${error.message}');
    if (error.docUrl != null) {
      stderr.writeln('Docs: ${error.docUrl}');
    }
    if (error.data != null) {
      stderr.writeln('Data: ${error.data}');
    }
    exitCode = 1;
  } finally {
    unsubscribe?.call();
    client.close();
  }
}
