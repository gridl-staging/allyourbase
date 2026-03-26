import 'package:test/test.dart';

import 'package:allyourbase/src/client.dart';
import 'package:allyourbase/src/errors.dart';

import 'support/deterministic_http_client.dart';

void main() {
  group('PushClient', () {
    late DeterministicHttpClient httpClient;
    late AYBClient client;

    setUp(() {
      httpClient = DeterministicHttpClient();
      client = AYBClient('https://example.com', httpClient: httpClient);
      client.setTokens('tok', 'ref');
    });

    group('registerDevice', () {
      test('sends POST to /api/push/devices with correct body', () async {
        httpClient.enqueue(StubResponse.json(200, {
          'id': 'dev-1',
          'provider': 'fcm',
          'platform': 'android',
          'token': 'fcm-token-abc',
          'device_name': 'Pixel 7',
          'is_active': true,
          'created_at': '2026-01-01T00:00:00Z',
        }));

        final result = await client.push.registerDevice(
          appId: 'app-1',
          provider: 'fcm',
          platform: 'android',
          token: 'fcm-token-abc',
          deviceName: 'Pixel 7',
        );

        expect(httpClient.requests, hasLength(1));
        final req = httpClient.requests.first;
        expect(req.method, 'POST');
        expect(req.url.path, '/api/push/devices');
        expect(req.headers['Authorization'], 'Bearer tok');

        final body = req.decodeJsonBody() as Map<String, Object?>;
        expect(body['app_id'], 'app-1');
        expect(body['provider'], 'fcm');
        expect(body['platform'], 'android');
        expect(body['token'], 'fcm-token-abc');
        expect(body['device_name'], 'Pixel 7');

        expect(result.id, 'dev-1');
        expect(result.provider, 'fcm');
        expect(result.token, 'fcm-token-abc');
        expect(result.isActive, isTrue);
      });

      test('omits device_name when not provided', () async {
        httpClient.enqueue(StubResponse.json(200, {
          'id': 'dev-2',
          'provider': 'apns',
          'platform': 'ios',
          'token': 'apns-token-xyz',
          'is_active': true,
          'created_at': '2026-01-01T00:00:00Z',
        }));

        await client.push.registerDevice(
          appId: 'app-1',
          provider: 'apns',
          platform: 'ios',
          token: 'apns-token-xyz',
        );

        final body =
            httpClient.requests.first.decodeJsonBody() as Map<String, Object?>;
        expect(body.containsKey('device_name'), isFalse);
      });

      test('propagates server errors', () async {
        httpClient.enqueue(StubResponse.json(400, {
          'message': 'token is required',
        }));

        expect(
          () => client.push.registerDevice(
            appId: 'app-1',
            provider: 'fcm',
            platform: 'android',
            token: '',
          ),
          throwsA(isA<AYBError>()
              .having((e) => e.status, 'status', 400)
              .having((e) => e.message, 'message', 'token is required')),
        );
      });
    });

    group('listDevices', () {
      test('sends GET to /api/push/devices with app_id query param', () async {
        httpClient.enqueue(StubResponse.json(200, {
          'items': [
            {
              'id': 'dev-1',
              'provider': 'fcm',
              'platform': 'android',
              'token': 'tok-1',
              'is_active': true,
              'created_at': '2026-01-01T00:00:00Z',
            },
            {
              'id': 'dev-2',
              'provider': 'apns',
              'platform': 'ios',
              'token': 'tok-2',
              'device_name': 'iPhone',
              'is_active': true,
              'created_at': '2026-01-02T00:00:00Z',
            },
          ],
        }));

        final devices = await client.push.listDevices('app-1');

        expect(httpClient.requests, hasLength(1));
        final req = httpClient.requests.first;
        expect(req.method, 'GET');
        expect(req.url.path, '/api/push/devices');
        expect(req.url.queryParameters['app_id'], 'app-1');
        expect(req.headers['Authorization'], 'Bearer tok');

        expect(devices, hasLength(2));
        expect(devices[0].id, 'dev-1');
        expect(devices[0].provider, 'fcm');
        expect(devices[1].id, 'dev-2');
        expect(devices[1].deviceName, 'iPhone');
      });

      test('returns empty list when no devices', () async {
        httpClient.enqueue(StubResponse.json(200, {
          'items': <Map<String, Object?>>[],
        }));

        final devices = await client.push.listDevices('app-1');
        expect(devices, isEmpty);
      });
    });

    group('revokeDevice', () {
      test('sends DELETE to /api/push/devices/{id}', () async {
        httpClient.enqueue(StubResponse.empty(204));

        await client.push.revokeDevice('dev-1');

        expect(httpClient.requests, hasLength(1));
        final req = httpClient.requests.first;
        expect(req.method, 'DELETE');
        expect(req.url.path, '/api/push/devices/dev-1');
        expect(req.headers['Authorization'], 'Bearer tok');
      });

      test('propagates 404 error', () async {
        httpClient.enqueue(StubResponse.json(404, {
          'message': 'Device not found',
        }));

        expect(
          () => client.push.revokeDevice('nonexistent'),
          throwsA(isA<AYBError>().having((e) => e.status, 'status', 404)),
        );
      });
    });

    test('works without auth (should fail with 401 from server)', () async {
      final noAuthClient =
          AYBClient('https://example.com', httpClient: httpClient);

      httpClient.enqueue(StubResponse.json(401, {
        'message': 'authentication required',
      }));

      expect(
        () => noAuthClient.push.listDevices('app-1'),
        throwsA(isA<AYBError>().having((e) => e.status, 'status', 401)),
      );
    });
  });
}
