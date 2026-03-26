import 'dart:convert';
import 'dart:typed_data';

import 'package:test/test.dart';

import 'package:allyourbase/src/client.dart';
import 'package:allyourbase/src/errors.dart';

import 'support/deterministic_http_client.dart';

void main() {
  group('StorageClient', () {
    late DeterministicHttpClient httpClient;
    late AYBClient client;

    setUp(() {
      httpClient = DeterministicHttpClient();
      client = AYBClient('https://example.com', httpClient: httpClient);
      client.setTokens('tok', 'ref');
    });

    group('upload', () {
      test('sends multipart POST to /api/storage/{bucket}', () async {
        httpClient.enqueue(StubResponse.json(200, {
          'id': 'file-1',
          'bucket': 'avatars',
          'name': 'photo.png',
          'size': 1024,
          'contentType': 'image/png',
          'userId': 'user-1',
          'createdAt': '2026-01-01T00:00:00Z',
          'updatedAt': '2026-01-01T00:00:00Z',
        }));

        final bytes = Uint8List.fromList([0x89, 0x50, 0x4E, 0x47]);
        final result = await client.storage.upload('avatars', bytes, 'photo.png');

        expect(httpClient.requests, hasLength(1));
        final req = httpClient.requests.first;
        expect(req.method, 'POST');
        expect(req.url.path, '/api/storage/avatars');
        // Auth header should be present
        expect(req.headers['Authorization'], 'Bearer tok');
        // Content-Type should contain multipart boundary
        expect(req.headers['content-type'], contains('multipart/form-data'));

        expect(result.id, 'file-1');
        expect(result.bucket, 'avatars');
        expect(result.name, 'photo.png');
        expect(result.size, 1024);
      });

      test('upload parses response into StorageObject', () async {
        httpClient.enqueue(StubResponse.json(200, {
          'id': 'file-2',
          'bucket': 'docs',
          'name': 'readme.md',
          'size': 256,
          'contentType': 'text/markdown',
          'createdAt': '2026-01-01T00:00:00Z',
          'updatedAt': '2026-01-01T00:00:00Z',
        }));

        final bytes = Uint8List.fromList(utf8.encode('# Hello'));
        final result = await client.storage.upload('docs', bytes, 'readme.md');

        expect(result.id, 'file-2');
        expect(result.name, 'readme.md');
        expect(result.contentType, 'text/markdown');
        expect(result.size, 256);
      });

      test('passes contentType to multipart file field', () async {
        httpClient.enqueue(StubResponse.json(200, {
          'id': 'file-3',
          'bucket': 'avatars',
          'name': 'photo.png',
          'size': 4,
          'contentType': 'image/png',
          'createdAt': '2026-01-01T00:00:00Z',
          'updatedAt': '2026-01-01T00:00:00Z',
        }));

        final bytes = Uint8List.fromList([0x89, 0x50, 0x4E, 0x47]);
        await client.storage
            .upload('avatars', bytes, 'photo.png', contentType: 'image/png');

        final req = httpClient.requests.first;
        // The multipart body should contain the specified content type
        // for the file part (not just application/octet-stream default)
        final body = req.body;
        expect(body, contains('image/png'));
      });

      test('upload propagates server errors', () async {
        httpClient.enqueue(StubResponse.json(413, {
          'message': 'File too large',
          'code': 'storage/file-too-large',
        }));

        final bytes = Uint8List(100);
        expect(
          () => client.storage.upload('avatars', bytes, 'big.bin'),
          throwsA(isA<AYBError>()
              .having((e) => e.status, 'status', 413)
              .having((e) => e.message, 'message', 'File too large')),
        );
      });

      test('upload error includes data and docUrl fields', () async {
        httpClient.enqueue(StubResponse.json(400, {
          'message': 'Invalid file',
          'code': 'storage/invalid-file',
          'data': {'field': 'file', 'reason': 'unsupported format'},
          'doc_url': 'https://docs.example.com/storage/upload',
        }));

        final bytes = Uint8List(10);
        expect(
          () => client.storage.upload('avatars', bytes, 'bad.xyz'),
          throwsA(isA<AYBError>()
              .having((e) => e.status, 'status', 400)
              .having((e) => e.message, 'message', 'Invalid file')
              .having((e) => e.code, 'code', 'storage/invalid-file')
              .having((e) => e.data, 'data', {'field': 'file', 'reason': 'unsupported format'})
              .having((e) => e.docUrl, 'docUrl', 'https://docs.example.com/storage/upload')),
        );
      });
    });

    group('downloadUrl', () {
      test('builds URL synchronously without HTTP call', () {
        final url = client.storage.downloadUrl('avatars', 'photo.png');
        expect(url, 'https://example.com/api/storage/avatars/photo.png');
        expect(httpClient.requests, isEmpty);
      });

      test('handles bucket and name with special characters', () {
        final url = client.storage.downloadUrl('my-bucket', 'path/to/file.pdf');
        expect(url, 'https://example.com/api/storage/my-bucket/path/to/file.pdf');
      });
    });

    group('delete', () {
      test('sends DELETE to /api/storage/{bucket}/{name}', () async {
        httpClient.enqueue(StubResponse.empty(204));

        await client.storage.delete('avatars', 'photo.png');

        expect(httpClient.requests, hasLength(1));
        final req = httpClient.requests.first;
        expect(req.method, 'DELETE');
        expect(req.url.path, '/api/storage/avatars/photo.png');
        expect(req.headers['Authorization'], 'Bearer tok');
      });

      test('delete propagates 404 error', () async {
        httpClient.enqueue(StubResponse.json(404, {
          'message': 'File not found',
          'code': 'storage/not-found',
        }));

        expect(
          () => client.storage.delete('avatars', 'missing.png'),
          throwsA(isA<AYBError>()
              .having((e) => e.status, 'status', 404)),
        );
      });
    });

    group('list', () {
      test('sends GET to /api/storage/{bucket}', () async {
        httpClient.enqueue(StubResponse.json(200, {
          'items': [
            {
              'id': 'f1',
              'bucket': 'avatars',
              'name': 'a.png',
              'size': 100,
              'contentType': 'image/png',
              'createdAt': '2026-01-01T00:00:00Z',
              'updatedAt': '2026-01-01T00:00:00Z',
            },
          ],
          'totalItems': 1,
        }));

        final result = await client.storage.list('avatars');

        expect(httpClient.requests, hasLength(1));
        final req = httpClient.requests.first;
        expect(req.method, 'GET');
        expect(req.url.path, '/api/storage/avatars');

        expect(result.items, hasLength(1));
        expect(result.items[0].name, 'a.png');
        expect(result.totalItems, 1);
      });

      test('passes prefix, limit, and offset query params', () async {
        httpClient.enqueue(StubResponse.json(200, {
          'items': <Map<String, Object?>>[],
          'totalItems': 0,
        }));

        await client.storage.list('docs',
            prefix: 'reports/', limit: 10, offset: 20);

        final req = httpClient.requests.first;
        expect(req.url.queryParameters['prefix'], 'reports/');
        expect(req.url.queryParameters['limit'], '10');
        expect(req.url.queryParameters['offset'], '20');
      });

      test('omits unset query params', () async {
        httpClient.enqueue(StubResponse.json(200, {
          'items': <Map<String, Object?>>[],
          'totalItems': 0,
        }));

        await client.storage.list('docs');

        final req = httpClient.requests.first;
        expect(req.url.queryParameters, isEmpty);
      });
    });

    group('getSignedUrl', () {
      test('sends POST to /api/storage/{bucket}/{name}/sign', () async {
        httpClient.enqueue(StubResponse.json(200, {
          'url': 'https://example.com/signed/abc123',
        }));

        final result = await client.storage.getSignedUrl('avatars', 'photo.png');

        expect(httpClient.requests, hasLength(1));
        final req = httpClient.requests.first;
        expect(req.method, 'POST');
        expect(req.url.path, '/api/storage/avatars/photo.png/sign');

        final body = req.decodeJsonBody() as Map<String, Object?>;
        expect(body['expiresIn'], 3600); // default

        expect(result, 'https://example.com/signed/abc123');
      });

      test('passes custom expiresIn', () async {
        httpClient.enqueue(StubResponse.json(200, {
          'url': 'https://example.com/signed/xyz',
        }));

        await client.storage.getSignedUrl('avatars', 'photo.png',
            expiresIn: 7200);

        final body = httpClient.requests.first.decodeJsonBody()
            as Map<String, Object?>;
        expect(body['expiresIn'], 7200);
      });

      test('propagates server errors', () async {
        httpClient.enqueue(StubResponse.json(403, {
          'message': 'Access denied',
          'code': 'storage/forbidden',
        }));

        expect(
          () => client.storage.getSignedUrl('private', 'secret.pdf'),
          throwsA(isA<AYBError>()
              .having((e) => e.status, 'status', 403)),
        );
      });
    });

    test('works with API key auth instead of JWT', () async {
      final apiClient = AYBClient('https://example.com', httpClient: httpClient);
      apiClient.setApiKey('ayb_test_key');

      httpClient.enqueue(StubResponse.empty(204));
      await apiClient.storage.delete('bucket', 'file.txt');

      final req = httpClient.requests.first;
      expect(req.headers['Authorization'], 'Bearer ayb_test_key');
    });
  });
}
