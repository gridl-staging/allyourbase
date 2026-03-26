import 'dart:async';

import 'package:test/test.dart';

import 'package:allyourbase/src/client.dart';
import 'package:allyourbase/src/types.dart';

import 'support/streaming_http_client.dart';

void main() {
  group('RealtimeClient', () {
    late StreamingHttpClient httpClient;
    late AYBClient client;

    setUp(() {
      httpClient = StreamingHttpClient();
      client = AYBClient('https://example.com', httpClient: httpClient);
    });

    test('subscribe connects to /api/realtime with tables query param',
        () async {
      final controller = httpClient.prepareStream(200);

      // subscribe will initiate the SSE connection
      final events = <RealtimeEvent>[];
      final unsub = client.realtime.subscribe(['posts', 'comments'], (event) {
        events.add(event);
      });

      // Wait for request to be sent
      await httpClient.waitForRequest();

      expect(httpClient.requests, hasLength(1));
      final req = httpClient.requests.first;
      expect(req.method, 'GET');
      expect(req.url.path, '/api/realtime');
      expect(req.url.queryParameters['tables'], 'posts,comments');

      // Send an SSE event
      controller.addSseEvent(
        data:
            '{"action":"INSERT","table":"posts","record":{"id":"1","title":"Hello"}}',
      );

      // Give stream processing a moment
      await Future<void>.delayed(const Duration(milliseconds: 50));

      expect(events, hasLength(1));
      expect(events[0].action, 'INSERT');
      expect(events[0].table, 'posts');
      expect(events[0].record['id'], '1');

      unsub();
      controller.close();
    });

    test('subscribe includes auth token in query params', () async {
      client.setTokens('test-token', 'test-refresh');
      final controller = httpClient.prepareStream(200);

      client.realtime.subscribe(['posts'], (_) {});
      await httpClient.waitForRequest();

      final req = httpClient.requests.first;
      expect(req.url.queryParameters['token'], 'test-token');

      controller.close();
    });

    test('subscribe does not include token when not authenticated', () async {
      final controller = httpClient.prepareStream(200);

      client.realtime.subscribe(['posts'], (_) {});
      await httpClient.waitForRequest();

      final req = httpClient.requests.first;
      expect(req.url.queryParameters.containsKey('token'), isFalse);

      controller.close();
    });

    test('unsubscribe stops processing events', () async {
      final controller = httpClient.prepareStream(200);
      final events = <RealtimeEvent>[];

      final unsub = client.realtime.subscribe(['posts'], (event) {
        events.add(event);
      });
      await httpClient.waitForRequest();

      // Send one event before unsubscribe
      controller.addSseEvent(
        data: '{"action":"INSERT","table":"posts","record":{"id":"1"}}',
      );
      await Future<void>.delayed(const Duration(milliseconds: 50));
      expect(events, hasLength(1));

      // Unsubscribe
      unsub();
      await Future<void>.delayed(const Duration(milliseconds: 10));

      // Send another event after unsubscribe — should not be received
      controller.addSseEvent(
        data: '{"action":"UPDATE","table":"posts","record":{"id":"2"}}',
      );
      await Future<void>.delayed(const Duration(milliseconds: 50));

      // Still only 1 event
      expect(events, hasLength(1));
      controller.close();
    });

    test('ignores malformed SSE data (heartbeat/ping)', () async {
      final controller = httpClient.prepareStream(200);
      final events = <RealtimeEvent>[];

      client.realtime.subscribe(['posts'], (event) {
        events.add(event);
      });
      await httpClient.waitForRequest();

      // Send a non-JSON heartbeat
      controller.addSseEvent(data: 'ping');

      // Send a valid event
      controller.addSseEvent(
        data: '{"action":"UPDATE","table":"posts","record":{"id":"2"}}',
      );

      await Future<void>.delayed(const Duration(milliseconds: 50));

      // Only the valid event should be received
      expect(events, hasLength(1));
      expect(events[0].action, 'UPDATE');

      controller.close();
    });

    test('delivers multiple events in sequence', () async {
      final controller = httpClient.prepareStream(200);
      final events = <RealtimeEvent>[];

      client.realtime.subscribe(['posts'], (event) {
        events.add(event);
      });
      await httpClient.waitForRequest();

      controller.addSseEvent(
        data: '{"action":"INSERT","table":"posts","record":{"id":"1"}}',
      );
      controller.addSseEvent(
        data:
            '{"action":"UPDATE","table":"posts","record":{"id":"1","title":"Updated"}}',
      );
      controller.addSseEvent(
        data: '{"action":"DELETE","table":"posts","record":{"id":"1"}}',
      );

      await Future<void>.delayed(const Duration(milliseconds: 50));

      expect(events, hasLength(3));
      expect(events[0].action, 'INSERT');
      expect(events[1].action, 'UPDATE');
      expect(events[2].action, 'DELETE');

      controller.close();
    });

    test('reconnects after stream closes and resumes event delivery', () async {
      client = AYBClient(
        'https://example.com',
        httpClient: httpClient,
        realtimeOptions: const RealtimeOptions(
          reconnectDelays: [Duration.zero],
          jitterMax: Duration.zero,
        ),
      );
      final first = httpClient.prepareStream(200);
      final second = httpClient.prepareStream(200);
      final events = <RealtimeEvent>[];

      final unsub = client.realtime.subscribe(['posts'], (event) {
        events.add(event);
      });

      await httpClient.waitForRequestCount(1);
      first.addSseEvent(
        data: '{"action":"INSERT","table":"posts","record":{"id":"1"}}',
      );
      await Future<void>.delayed(const Duration(milliseconds: 20));

      first.close();

      await httpClient.waitForRequestCount(2);
      second.addSseEvent(
        data:
            '{"action":"UPDATE","table":"posts","record":{"id":"1","title":"After reconnect"}}',
      );
      await Future<void>.delayed(const Duration(milliseconds: 20));

      expect(events, hasLength(2));
      expect(events[0].action, 'INSERT');
      expect(events[1].action, 'UPDATE');

      unsub();
      second.close();
    });

    test('reconnect uses the latest token value', () async {
      client = AYBClient(
        'https://example.com',
        httpClient: httpClient,
        realtimeOptions: const RealtimeOptions(
          reconnectDelays: [Duration.zero],
          jitterMax: Duration.zero,
        ),
      );
      client.setTokens('token-one', 'refresh-one');
      final first = httpClient.prepareStream(200);
      final second = httpClient.prepareStream(200);

      final unsub = client.realtime.subscribe(['posts'], (_) {});
      await httpClient.waitForRequestCount(1);
      expect(httpClient.requests[0].url.queryParameters['token'], 'token-one');

      client.setTokens('token-two', 'refresh-two');
      first.close();

      await httpClient.waitForRequestCount(2);
      expect(httpClient.requests[1].url.queryParameters['token'], 'token-two');

      unsub();
      second.close();
    });

    test('stops reconnecting after max retry attempts', () async {
      client = AYBClient(
        'https://example.com',
        httpClient: httpClient,
        realtimeOptions: const RealtimeOptions(
          maxReconnectAttempts: 2,
          reconnectDelays: [Duration.zero],
          jitterMax: Duration.zero,
        ),
      );

      // Prepare more streams than max retries to verify the cap stops
      // reconnection — not just running out of prepared streams.
      httpClient.prepareStream(503);
      httpClient.prepareStream(503);
      httpClient.prepareStream(503);
      httpClient.prepareStream(503);
      httpClient.prepareStream(503);

      client.realtime.subscribe(['posts'], (_) {});

      // 1 initial + 2 retries = 3 total requests.
      await httpClient.waitForRequestCount(3);
      await Future<void>.delayed(const Duration(milliseconds: 50));
      expect(httpClient.requests, hasLength(3));
    });

    test('supports empty reconnectDelays by falling back to zero delay',
        () async {
      client = AYBClient(
        'https://example.com',
        httpClient: httpClient,
        realtimeOptions: const RealtimeOptions(
          maxReconnectAttempts: 2,
          reconnectDelays: [],
          jitterMax: Duration.zero,
        ),
      );

      httpClient.prepareStream(503);
      httpClient.prepareStream(503);
      httpClient.prepareStream(503);

      client.realtime.subscribe(['posts'], (_) {});

      // 1 initial + 2 retries = 3 total requests.
      await httpClient.waitForRequestCount(3);
      expect(httpClient.requests, hasLength(3));
    });

    test('unsubscribe prevents reconnect after intentional close', () async {
      client = AYBClient(
        'https://example.com',
        httpClient: httpClient,
        realtimeOptions: const RealtimeOptions(
          reconnectDelays: [Duration.zero],
          jitterMax: Duration.zero,
        ),
      );
      final first = httpClient.prepareStream(200);
      httpClient.prepareStream(200);

      final unsub = client.realtime.subscribe(['posts'], (_) {});
      await httpClient.waitForRequestCount(1);

      unsub();
      first.close();

      await Future<void>.delayed(const Duration(milliseconds: 50));
      expect(httpClient.requests, hasLength(1));
    });

    test('applies stepped reconnect delays with jitter', () async {
      final recordedDelays = <Duration>[];
      client = AYBClient(
        'https://example.com',
        httpClient: httpClient,
        realtimeOptions: RealtimeOptions(
          maxReconnectAttempts: 2,
          reconnectDelays: const [
            Duration(milliseconds: 200),
            Duration(milliseconds: 500),
          ],
          jitterMax: const Duration(milliseconds: 100),
          randomDouble: () => 0.5,
          sleep: (delay) async {
            recordedDelays.add(delay);
          },
        ),
      );

      httpClient.prepareStream(503);
      httpClient.prepareStream(503);
      httpClient.prepareStream(503);

      client.realtime.subscribe(['posts'], (_) {});
      await httpClient.waitForRequestCount(3);

      expect(recordedDelays, [
        const Duration(milliseconds: 250),
        const Duration(milliseconds: 550),
      ]);
    });
  });
}
