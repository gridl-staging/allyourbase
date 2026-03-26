import 'dart:convert';

import 'package:test/test.dart';

import 'package:allyourbase/src/client.dart';
import 'package:allyourbase/src/errors.dart';

import 'support/streaming_http_client.dart';

void main() {
  group('AuthClient.signInWithOAuth', () {
    late StreamingHttpClient httpClient;
    late AYBClient client;

    setUp(() {
      httpClient = StreamingHttpClient();
      client = AYBClient('https://example.com', httpClient: httpClient);
    });

    test('connects to SSE, calls urlCallback, stores tokens on oauth event',
        () async {
      final controller = httpClient.prepareStream(200);
      String? capturedUrl;

      final future = client.auth.signInWithOAuth(
        'google',
        urlCallback: (url) async {
          capturedUrl = url;
          // Simulate the OAuth redirect completing — backend sends tokens via SSE
          controller.addSseEvent(
            event: 'oauth',
            data: jsonEncode({
              'token': 'oauth-token',
              'refreshToken': 'oauth-refresh',
            }),
          );
        },
      );

      await httpClient.waitForRequest();

      // SSE should connect to /api/realtime?oauth=true
      final req = httpClient.requests.first;
      expect(req.url.path, '/api/realtime');
      expect(req.url.queryParameters['oauth'], 'true');

      // Send connected event with clientId
      controller.addSseEvent(
        event: 'connected',
        data: jsonEncode({'clientId': 'client-123'}),
      );

      final result = await future;

      // Verify urlCallback was called with correct OAuth URL
      expect(capturedUrl,
          'https://example.com/api/auth/oauth/google?state=client-123');

      // Verify tokens stored
      expect(result.token, 'oauth-token');
      expect(result.refreshToken, 'oauth-refresh');
      expect(client.token, 'oauth-token');
      expect(client.refreshToken, 'oauth-refresh');
    });

    test('includes scopes in OAuth URL', () async {
      final controller = httpClient.prepareStream(200);
      String? capturedUrl;

      final future = client.auth.signInWithOAuth(
        'github',
        scopes: ['read:user', 'user:email'],
        urlCallback: (url) async {
          capturedUrl = url;
          controller.addSseEvent(
            event: 'oauth',
            data: jsonEncode({
              'token': 't',
              'refreshToken': 'r',
            }),
          );
        },
      );

      await httpClient.waitForRequest();

      controller.addSseEvent(
        event: 'connected',
        data: jsonEncode({'clientId': 'c-456'}),
      );

      await future;

      expect(capturedUrl, contains('state=c-456'));
      expect(capturedUrl, contains('scopes=read%3Auser%2Cuser%3Aemail'));
    });

    test('emits SIGNED_IN auth event', () async {
      final controller = httpClient.prepareStream(200);
      final events = <String>[];
      client.onAuthStateChange((event, session) {
        events.add(event);
      });

      final future = client.auth.signInWithOAuth(
        'google',
        urlCallback: (url) async {
          controller.addSseEvent(
            event: 'oauth',
            data: jsonEncode({
              'token': 't',
              'refreshToken': 'r',
            }),
          );
        },
      );

      await httpClient.waitForRequest();
      controller.addSseEvent(
        event: 'connected',
        data: jsonEncode({'clientId': 'c-1'}),
      );

      await future;
      expect(events, ['SIGNED_IN']);
    });

    test('throws AYBError on provider error in oauth event', () async {
      final controller = httpClient.prepareStream(200);

      final future = client.auth.signInWithOAuth(
        'google',
        urlCallback: (url) async {
          controller.addSseEvent(
            event: 'oauth',
            data: jsonEncode({'error': 'access_denied'}),
          );
        },
      );

      await httpClient.waitForRequest();
      controller.addSseEvent(
        event: 'connected',
        data: jsonEncode({'clientId': 'c-1'}),
      );

      expect(
        future,
        throwsA(isA<AYBError>()
            .having((e) => e.status, 'status', 401)
            .having((e) => e.code, 'code', 'oauth/provider-error')
            .having((e) => e.message, 'message', 'access_denied')),
      );
    });

    test('throws AYBError when SSE connection fails', () async {
      httpClient.prepareStream(503);

      expect(
        client.auth.signInWithOAuth(
          'google',
          urlCallback: (_) async {},
        ),
        throwsA(isA<AYBError>()
            .having((e) => e.status, 'status', 503)
            .having((e) => e.code, 'code', 'oauth/sse-failed')),
      );
    });

    test('throws when oauth event has missing tokens', () async {
      final controller = httpClient.prepareStream(200);

      final future = client.auth.signInWithOAuth(
        'google',
        urlCallback: (url) async {
          controller.addSseEvent(
            event: 'oauth',
            data: jsonEncode({'token': 'only-access-token'}),
          );
        },
      );

      await httpClient.waitForRequest();
      controller.addSseEvent(
        event: 'connected',
        data: jsonEncode({'clientId': 'c-1'}),
      );

      expect(
        future,
        throwsA(isA<AYBError>()
            .having((e) => e.code, 'code', 'oauth/missing-tokens')),
      );
    });

    test('throws when SSE stream closes before oauth event', () async {
      final controller = httpClient.prepareStream(200);

      final future = client.auth.signInWithOAuth(
        'google',
        urlCallback: (url) async {
          // Don't send any oauth event — just close the stream
          controller.close();
        },
      );

      await httpClient.waitForRequest();
      controller.addSseEvent(
        event: 'connected',
        data: jsonEncode({'clientId': 'c-1'}),
      );

      expect(
        future,
        throwsA(isA<AYBError>()
            .having((e) => e.code, 'code', 'oauth/sse-closed')),
      );
    });

    test('throws when connected event has empty data', () async {
      final controller = httpClient.prepareStream(200);

      final future = client.auth.signInWithOAuth(
        'google',
        urlCallback: (_) async {},
      );

      await httpClient.waitForRequest();
      controller.addSseEvent(event: 'connected', data: '');

      expect(
        future,
        throwsA(isA<AYBError>()
            .having((e) => e.code, 'code', 'oauth/invalid-connected-event')),
      );
    });

    test('throws when connected event has invalid JSON', () async {
      final controller = httpClient.prepareStream(200);

      final future = client.auth.signInWithOAuth(
        'google',
        urlCallback: (_) async {},
      );

      await httpClient.waitForRequest();
      controller.addSseEvent(event: 'connected', data: 'not-json{');

      expect(
        future,
        throwsA(isA<AYBError>()
            .having((e) => e.code, 'code', 'oauth/invalid-connected-event')),
      );
    });

    test('throws when connected event has missing clientId', () async {
      final controller = httpClient.prepareStream(200);

      final future = client.auth.signInWithOAuth(
        'google',
        urlCallback: (_) async {},
      );

      await httpClient.waitForRequest();
      controller.addSseEvent(
        event: 'connected',
        data: jsonEncode({'someOtherField': 'value'}),
      );

      expect(
        future,
        throwsA(isA<AYBError>()
            .having((e) => e.code, 'code', 'oauth/invalid-connected-event')),
      );
    });

    test('throws when oauth event has invalid JSON', () async {
      final controller = httpClient.prepareStream(200);

      final future = client.auth.signInWithOAuth(
        'google',
        urlCallback: (url) async {
          controller.addSseEvent(event: 'oauth', data: '{broken');
        },
      );

      await httpClient.waitForRequest();
      controller.addSseEvent(
        event: 'connected',
        data: jsonEncode({'clientId': 'c-1'}),
      );

      expect(
        future,
        throwsA(isA<AYBError>()
            .having((e) => e.code, 'code', 'oauth/invalid-oauth-event')),
      );
    });

    test('propagates synchronous urlCallback errors', () async {
      final controller = httpClient.prepareStream(200);

      final future = client.auth.signInWithOAuth(
        'google',
        urlCallback: (_) {
          throw StateError('launch failed');
        },
      );

      await httpClient.waitForRequest();
      controller.addSseEvent(
        event: 'connected',
        data: jsonEncode({'clientId': 'c-1'}),
      );

      await expectLater(
        future,
        throwsA(isA<StateError>()
            .having((e) => e.message, 'message', 'launch failed')),
      );
      expect(client.token, isNull);
      expect(client.refreshToken, isNull);
      controller.close();
    });
  });
}
