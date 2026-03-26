import 'package:allyourbase/allyourbase.dart';
import 'package:test/test.dart';

import 'support/deterministic_http_client.dart';

void main() {
  group('AYBClient', () {
    test('normalizes trailing slashes in baseUrl', () {
      final client = AYBClient('https://api.example.com///');

      expect(client.baseUrl, 'https://api.example.com');
    });

    test('exposes sdk sub-clients', () {
      final client = AYBClient('https://api.example.com');

      expect(client.auth, isA<AuthClient>());
      expect(client.records, isA<RecordsClient>());
      expect(client.storage, isA<StorageClient>());
      expect(client.realtime, isA<RealtimeClient>());
      expect(client.push, isA<PushClient>());
      expect(identical(client.auth.client, client), isTrue);
      expect(identical(client.records.client, client), isTrue);
      expect(identical(client.storage.client, client), isTrue);
      expect(identical(client.realtime.client, client), isTrue);
      expect(identical(client.push.client, client), isTrue);
    });

    test('uses injected http client instance', () {
      final injected = DeterministicHttpClient();

      final client = AYBClient(
        'https://api.example.com',
        httpClient: injected,
      );

      expect(identical(client.httpClient, injected), isTrue);
    });

    test('setTokens and clearTokens update token state', () {
      final client = AYBClient('https://api.example.com');

      client.setTokens('tok_123', 'refresh_123');
      expect(client.token, 'tok_123');
      expect(client.refreshToken, 'refresh_123');

      client.clearTokens();
      expect(client.token, isNull);
      expect(client.refreshToken, isNull);
    });

    test('setApiKey stores key as token and clears refresh token', () {
      final client = AYBClient('https://api.example.com');
      client.setTokens('jwt_123', 'refresh_123');

      client.setApiKey('ayb_key_123');

      expect(client.token, 'ayb_key_123');
      expect(client.refreshToken, isNull);
    });

    test('clearApiKey clears token state', () {
      final client = AYBClient('https://api.example.com');
      client.setApiKey('ayb_key_123');

      client.clearApiKey();

      expect(client.token, isNull);
      expect(client.refreshToken, isNull);
    });

    test('listener that unsubscribes itself during emit does not crash', () {
      final client = AYBClient('https://api.example.com');
      client.setTokens('tok', 'ref');

      final events = <String>[];
      late void Function() unsub;
      unsub = client.onAuthStateChange((event, session) {
        events.add(event);
        unsub(); // unsubscribe during iteration
      });

      // Should not throw ConcurrentModificationError.
      client.emitAuthEvent(AuthStateEvent.signedIn);

      expect(events, ['SIGNED_IN']);
    });

    test('close() closes the underlying http client', () async {
      final http = DeterministicHttpClient([
        StubResponse.json(200, const {'ok': true}),
      ]);
      final client = AYBClient('https://api.example.com', httpClient: http);

      // Verify the client works before close.
      await client.request<Map<String, Object?>>('/api/test');
      expect(http.requests, hasLength(1));

      // close() should not throw.
      client.close();
    });
  });

  group('request', () {
    test('injects bearer auth header when token is set', () async {
      final http = DeterministicHttpClient([
        StubResponse.json(200, const {'ok': true}),
      ]);
      final client = AYBClient('https://api.example.com', httpClient: http);
      client.setTokens('jwt_abc', 'refresh_abc');

      final response = await client.request<Map<String, Object?>>('/api/me');

      expect(response['ok'], isTrue);
      expect(http.requests, hasLength(1));
      expect(http.requests.single.url.toString(), 'https://api.example.com/api/me');
      expect(_header(http.requests.single.headers, 'Authorization'), 'Bearer jwt_abc');
    });

    test('skipAuth omits auth header even when token exists', () async {
      final http = DeterministicHttpClient([
        StubResponse.json(200, const {'ok': true}),
      ]);
      final client = AYBClient('https://api.example.com', httpClient: http);
      client.setTokens('jwt_abc', 'refresh_abc');

      await client.request<Map<String, Object?>>('/api/public', skipAuth: true);

      expect(_header(http.requests.single.headers, 'Authorization'), isNull);
    });

    test('encodes json request bodies and sets content type', () async {
      final http = DeterministicHttpClient([
        StubResponse.json(200, const {'ok': true}),
      ]);
      final client = AYBClient('https://api.example.com', httpClient: http);

      await client.request<Map<String, Object?>>(
        '/api/echo',
        method: 'POST',
        body: const {'a': 1, 'b': 'two'},
      );

      final req = http.requests.single;
      expect(req.method, 'POST');
      expect(_header(req.headers, 'Content-Type'), 'application/json');
      expect(req.decodeJsonBody(), const {'a': 1, 'b': 'two'});
    });

    test('returns null for 204 responses', () async {
      final http = DeterministicHttpClient([StubResponse.empty(204)]);
      final client = AYBClient('https://api.example.com', httpClient: http);

      final result = await client.request<Map<String, Object?>?>(
        '/api/items/1',
        method: 'DELETE',
      );

      expect(result, isNull);
      expect(http.requests.single.method, 'DELETE');
    });

    test('throws AYBError with normalized server fields', () async {
      final http = DeterministicHttpClient([
        StubResponse.json(409, const {
          'message': 'unique violation',
          'code': 'db/unique',
          'data': {
            'users_email_key': {'code': 'unique_violation'},
          },
          'doc_url': 'https://allyourbase.io/guide/errors#db-unique',
        }),
      ]);
      final client = AYBClient('https://api.example.com', httpClient: http);

      final matcher = throwsA(
        isA<AYBError>()
            .having((e) => e.status, 'status', 409)
            .having((e) => e.message, 'message', 'unique violation')
            .having((e) => e.code, 'code', 'db/unique')
            .having(
              (e) => e.data,
              'data',
              {'users_email_key': {'code': 'unique_violation'}},
            )
            .having(
              (e) => e.docUrl,
              'docUrl',
              'https://allyourbase.io/guide/errors#db-unique',
            ),
      );
      await expectLater(
        () => client.request<void>('/api/fail'),
        matcher,
      );
    });

    test('uses status text when error body is not valid json', () async {
      final http = DeterministicHttpClient([
        StubResponse.text(
          502,
          '<!doctype html>gateway',
          reasonPhrase: 'Bad Gateway',
        ),
      ]);
      final client = AYBClient('https://api.example.com', httpClient: http);

      await expectLater(
        () => client.request<void>('/api/fail'),
        throwsA(
          isA<AYBError>()
              .having((e) => e.status, 'status', 502)
              .having((e) => e.message, 'message', 'Bad Gateway'),
        ),
      );
    });
  });

  group('rpc', () {
    test('posts args payload as json when args are present', () async {
      final http = DeterministicHttpClient([StubResponse.json(200, 42)]);
      final client = AYBClient('https://api.example.com', httpClient: http);

      final value = await client.rpc<int>('get_total', args: const {'user_id': 'abc'});

      expect(value, 42);
      final req = http.requests.single;
      expect(req.method, 'POST');
      expect(req.url.toString(), 'https://api.example.com/api/rpc/get_total');
      expect(_header(req.headers, 'Content-Type'), 'application/json');
      expect(req.decodeJsonBody(), const {'user_id': 'abc'});
    });

    test('omits body and content type when args are null', () async {
      final http = DeterministicHttpClient([StubResponse.json(200, 'ok')]);
      final client = AYBClient('https://api.example.com', httpClient: http);

      final result = await client.rpc<String>('no_args_fn');

      expect(result, 'ok');
      final req = http.requests.single;
      expect(req.bodyBytes, isEmpty);
      expect(_header(req.headers, 'Content-Type'), isNull);
    });

    test('omits body and content type when args are empty map', () async {
      final http = DeterministicHttpClient([StubResponse.json(200, 'ok')]);
      final client = AYBClient('https://api.example.com', httpClient: http);

      await client.rpc<String>('no_args_fn', args: const <String, Object?>{});

      final req = http.requests.single;
      expect(req.bodyBytes, isEmpty);
      expect(_header(req.headers, 'Content-Type'), isNull);
    });

    test('propagates auth bearer header for rpc calls', () async {
      final http = DeterministicHttpClient([StubResponse.json(200, 1)]);
      final client = AYBClient('https://api.example.com', httpClient: http);
      client.setTokens('jwt_abc', 'refresh_abc');

      await client.rpc<int>('my_fn');

      expect(_header(http.requests.single.headers, 'Authorization'), 'Bearer jwt_abc');
    });
  });
}

String? _header(Map<String, String> headers, String key) {
  for (final entry in headers.entries) {
    if (entry.key.toLowerCase() == key.toLowerCase()) {
      return entry.value;
    }
  }
  return null;
}
