// Contract tests: verify Dart SDK types parse the exact JSON shapes
// returned by the AYB Go server. Fixtures match Go struct JSON tags.
import 'package:test/test.dart';

import 'package:allyourbase/src/client.dart';
import 'package:allyourbase/src/errors.dart';
import 'package:allyourbase/src/types.dart';

import 'support/deterministic_http_client.dart';

void main() {
  group('Contract: auth responses', () {
    test('AuthResponse matches server login/register shape', () {
      // Canonical fixture: auth_response.json
      final json = <String, Object?>{
        'token': 'jwt_stage3',
        'refreshToken': 'refresh_stage3',
        'user': {
          'id': 'usr_1',
          'email': 'dev@allyourbase.io',
          'email_verified': true,
          'created_at': '2026-01-01T00:00:00Z',
          'updated_at': null,
        },
      };

      final auth = AuthResponse.fromJson(json);
      expect(auth.token, 'jwt_stage3');
      expect(auth.refreshToken, 'refresh_stage3');
      expect(auth.user.id, 'usr_1');
      expect(auth.user.email, 'dev@allyourbase.io');
      expect(auth.user.emailVerified, isTrue);
      expect(auth.user.createdAt, '2026-01-01T00:00:00Z');
      expect(auth.user.updatedAt, isNull);
    });

    test('User parses server shape with minimal fields', () {
      // Server can omit optional fields (phone, emailVerified)
      final json = <String, Object?>{
        'id': 'usr_2',
        'email': 'bob@example.com',
        'createdAt': '2026-02-22T10:00:00Z',
        'updatedAt': '2026-02-22T10:00:00Z',
      };

      final user = User.fromJson(json);
      expect(user.id, 'usr_2');
      expect(user.email, 'bob@example.com');
    });
  });

  group('Contract: error responses', () {
    test('AYBError parses server ErrorResponse with doc_url (snake_case)', () async {
      // Canonical fixture: error_response_numeric_code.json
      final http = DeterministicHttpClient();
      final client = AYBClient('https://api.example.com', httpClient: http);
      http.enqueue(StubResponse.json(403, {
        'code': 403,
        'message': 'forbidden',
        'data': {'resource': 'posts'},
        'doc_url': 'https://allyourbase.io/docs/errors#forbidden',
      }));

      try {
        await client.request<Map<String, Object?>>('/api/auth/login', method: 'POST');
        fail('Expected AYBError');
      } on AYBError catch (e) {
        expect(e.status, 403);
        expect(e.message, 'forbidden');
        expect(e.code, '403');
        expect(e.data, isA<Map<String, Object?>>());
        expect(e.data!['resource'], 'posts');
        expect(e.docUrl, 'https://allyourbase.io/docs/errors#forbidden');
      }
    });

    test('AYBError parses server ErrorResponse with string code', () async {
      // Canonical fixture: error_response_string_code.json
      final http = DeterministicHttpClient();
      final client = AYBClient('https://api.example.com', httpClient: http);
      http.enqueue(StubResponse.json(400, {
        'code': 'auth/missing-refresh-token',
        'message': 'Missing refresh token',
        'data': {'detail': 'refresh token not available'},
      }));

      try {
        await client.request<Map<String, Object?>>('/api/auth/refresh', method: 'POST');
        fail('Expected AYBError');
      } on AYBError catch (e) {
        expect(e.status, 400);
        expect(e.message, 'Missing refresh token');
        expect(e.code, 'auth/missing-refresh-token');
        expect(e.data!['detail'], 'refresh token not available');
        expect(e.docUrl, isNull);
      }
    });
  });

  group('Contract: collection list response', () {
    test('ListResponse matches server paginated shape', () {
      // Canonical fixture: list_response.json
      final json = <String, Object?>{
        'page': 1,
        'perPage': 2,
        'totalItems': 2,
        'totalPages': 1,
        'items': [
          {'id': 'rec_1', 'title': 'First'},
          {'id': 'rec_2', 'title': 'Second'},
        ],
      };

      final response = ListResponse<Map<String, Object?>>.fromJson(
        json,
        decodeItem: (v) => Map<String, Object?>.from(v as Map),
      );
      expect(response.page, 1);
      expect(response.perPage, 2);
      expect(response.totalItems, 2);
      expect(response.totalPages, 1);
      expect(response.items, hasLength(2));
      expect(response.items[0]['id'], 'rec_1');
      expect(response.items[0]['title'], 'First');
      expect(response.items[1]['title'], 'Second');
    });
  });

  group('Contract: batch response', () {
    test('BatchResult matches server shape', () {
      // Go: BatchResult{Index int, Status int, Body map[string]any}
      final json = <String, Object?>{
        'index': 0,
        'status': 201,
        'body': {'id': 'rec_new', 'title': 'Created'},
      };

      final result = BatchResult<Map<String, Object?>>.fromJson(
        json,
        decodeBody: (v) => Map<String, Object?>.from(v as Map),
      );
      expect(result.index, 0);
      expect(result.status, 201);
      expect(result.body?['id'], 'rec_new');
    });

    test('BatchResult handles 204 delete (null body)', () {
      final json = <String, Object?>{
        'index': 2,
        'status': 204,
      };

      final result = BatchResult<Object>.fromJson(json);
      expect(result.index, 2);
      expect(result.status, 204);
      expect(result.body, isNull);
    });
  });

  group('Contract: storage responses', () {
    test('StorageObject matches server shape', () {
      // Canonical fixture: storage_object.json
      final json = <String, Object?>{
        'id': 'file_abc123',
        'bucket': 'uploads',
        'name': 'document.pdf',
        'size': 1024,
        'contentType': 'application/pdf',
        'userId': 'usr_1',
        'createdAt': '2026-01-01T00:00:00Z',
        'updatedAt': '2026-01-02T12:30:00Z',
      };

      final obj = StorageObject.fromJson(json);
      expect(obj.id, 'file_abc123');
      expect(obj.bucket, 'uploads');
      expect(obj.name, 'document.pdf');
      expect(obj.size, 1024);
      expect(obj.contentType, 'application/pdf');
      expect(obj.userId, 'usr_1');
      expect(obj.createdAt, '2026-01-01T00:00:00Z');
      expect(obj.updatedAt, '2026-01-02T12:30:00Z');
    });

    test('StorageListResponse matches canonical list shape with nullable fields', () {
      final json = <String, Object?>{
        'items': [
          {
            'id': 'file_1',
            'bucket': 'uploads',
            'name': 'doc1.pdf',
            'size': 1024,
            'contentType': 'application/pdf',
            'userId': 'usr_1',
            'createdAt': '2026-01-01T00:00:00Z',
            'updatedAt': null,
          },
          {
            'id': 'file_2',
            'bucket': 'uploads',
            'name': 'image.png',
            'size': 2048,
            'contentType': 'image/png',
            'userId': null,
            'createdAt': '2026-01-02T00:00:00Z',
            'updatedAt': null,
          },
        ],
        'totalItems': 2,
      };

      final response = StorageListResponse.fromJson(json);
      expect(response.totalItems, 2);
      expect(response.items, hasLength(2));
      expect(response.items[0].userId, 'usr_1');
      expect(response.items[0].updatedAt, isNull);
      expect(response.items[1].userId, isNull);
      expect(response.items[1].updatedAt, isNull);
    });
  });

  group('Contract: push device token responses', () {
    test('DeviceToken matches server snake_case JSON shape', () {
      // Go: DeviceToken uses snake_case JSON tags:
      //   app_id, user_id, device_name, is_active, last_used, last_refreshed_at, created_at, updated_at
      final json = <String, Object?>{
        'id': 'dt_1',
        'app_id': 'app_main',
        'user_id': 'usr_1',
        'provider': 'fcm',
        'platform': 'android',
        'token': 'fcm-token-xyz',
        'device_name': 'Pixel 9 Pro',
        'is_active': true,
        'last_refreshed_at': '2026-02-22T12:00:00Z',
        'created_at': '2026-02-22T10:00:00Z',
        'updated_at': '2026-02-22T12:00:00Z',
      };

      final dt = DeviceToken.fromJson(json);
      expect(dt.id, 'dt_1');
      expect(dt.provider, 'fcm');
      expect(dt.platform, 'android');
      expect(dt.token, 'fcm-token-xyz');
      expect(dt.deviceName, 'Pixel 9 Pro');
      expect(dt.isActive, isTrue);
      expect(dt.lastRefreshedAt, '2026-02-22T12:00:00Z');
      expect(dt.createdAt, '2026-02-22T10:00:00Z');
    });

    test('DeviceToken handles null optional fields', () {
      final json = <String, Object?>{
        'id': 'dt_2',
        'app_id': 'app_main',
        'user_id': 'usr_1',
        'provider': 'apns',
        'platform': 'ios',
        'token': 'apns-token-abc',
        'is_active': true,
        'created_at': '2026-02-22T10:00:00Z',
        'updated_at': '2026-02-22T10:00:00Z',
      };

      final dt = DeviceToken.fromJson(json);
      expect(dt.deviceName, isNull);
      expect(dt.lastRefreshedAt, isNull);
    });
  });

  group('Contract: GeoJSON round-trip', () {
    test('Record with GeoJSON Point location round-trips through create/get', () async {
      // Server returns geometry columns as GeoJSON objects (ST_AsGeoJSON wrapping).
      // The SDK uses Map<String, Object?> for records — GeoJSON is a plain map.
      final geoPoint = <String, Object?>{
        'type': 'Point',
        'coordinates': [-73.9654, 40.7829],
      };

      final http = DeterministicHttpClient();
      final client = AYBClient('https://api.example.com', httpClient: http);
      client.setTokens('test-token', 'test-refresh');

      // Stub create response (server returns the record with GeoJSON)
      http.enqueue(StubResponse.json(201, {
        'id': 'place_1',
        'name': 'Central Park',
        'location': geoPoint,
        'boundary': null,
        'created_at': '2026-02-22T10:00:00Z',
      }));

      final created = await client.records.create('places', {
        'name': 'Central Park',
        'location': geoPoint,
      });

      expect(http.requests, hasLength(1));
      expect(http.requests.first.method, 'POST');
      expect(http.requests.first.url.path, '/api/collections/places');
      expect(created['id'], 'place_1');
      expect(created['name'], 'Central Park');
      expect(created['location'], isA<Map<String, Object?>>());
      final loc = created['location'] as Map<String, Object?>;
      expect(loc['type'], 'Point');
      expect(loc['coordinates'], [-73.9654, 40.7829]);
      expect(created['boundary'], isNull);

      // Verify the request body sent the GeoJSON as-is
      final reqBody = http.requests.first.decodeJsonBody() as Map<String, Object?>;
      expect(reqBody['location'], isA<Map<String, Object?>>());
      final sentLoc = reqBody['location'] as Map<String, Object?>;
      expect(sentLoc['type'], 'Point');
      expect(sentLoc['coordinates'], [-73.9654, 40.7829]);

      // Stub get response
      http.enqueue(StubResponse.json(200, {
        'id': 'place_1',
        'name': 'Central Park',
        'location': geoPoint,
        'boundary': null,
        'created_at': '2026-02-22T10:00:00Z',
      }));

      final fetched = await client.records.get('places', 'place_1');
      expect(http.requests, hasLength(2));
      expect(http.requests[1].method, 'GET');
      expect(http.requests[1].url.path, '/api/collections/places/place_1');
      expect(fetched['location'], isA<Map<String, Object?>>());
      final fetchedLoc = fetched['location'] as Map<String, Object?>;
      expect(fetchedLoc['type'], 'Point');
      expect(fetchedLoc['coordinates'], [-73.9654, 40.7829]);
    });

    test('Record with GeoJSON Polygon round-trips correctly', () async {
      final polygon = <String, Object?>{
        'type': 'Polygon',
        'coordinates': [
          [[-73.9, 40.7], [-73.8, 40.7], [-73.8, 40.8], [-73.9, 40.8], [-73.9, 40.7]],
        ],
      };

      final http = DeterministicHttpClient();
      final client = AYBClient('https://api.example.com', httpClient: http);
      client.setTokens('test-token', 'test-refresh');

      http.enqueue(StubResponse.json(201, {
        'id': 'zone_1',
        'name': 'Manhattan',
        'boundary': polygon,
        'created_at': '2026-02-22T10:00:00Z',
      }));

      final created = await client.records.create('zones', {
        'name': 'Manhattan',
        'boundary': polygon,
      });

      expect(http.requests, hasLength(1));
      expect(http.requests.first.method, 'POST');
      expect(http.requests.first.url.path, '/api/collections/zones');
      final reqBody = http.requests.first.decodeJsonBody() as Map<String, Object?>;
      final sentBoundary = reqBody['boundary'] as Map<String, Object?>;
      expect(sentBoundary['type'], 'Polygon');
      final sentCoords = sentBoundary['coordinates'] as List;
      expect(sentCoords, hasLength(1)); // one ring
      expect((sentCoords[0] as List), hasLength(5)); // 5 points (closed ring)

      final bnd = created['boundary'] as Map<String, Object?>;
      expect(bnd['type'], 'Polygon');
      final coords = bnd['coordinates'] as List;
      expect(coords, hasLength(1)); // one ring
      expect((coords[0] as List), hasLength(5)); // 5 points (closed ring)
    });

    test('RealtimeEvent with GeoJSON in record payload', () {
      // SSE events for spatial tables include GeoJSON in the record map
      final json = <String, Object?>{
        'action': 'INSERT',
        'table': 'places',
        'record': {
          'id': 'place_1',
          'name': 'Central Park',
          'location': {
            'type': 'Point',
            'coordinates': [-73.9654, 40.7829],
          },
        },
      };

      final event = RealtimeEvent.fromJson(json);
      expect(event.action, 'INSERT');
      expect(event.table, 'places');
      expect(event.record['location'], isA<Map<String, Object?>>());
      final loc = event.record['location'] as Map<String, Object?>;
      expect(loc['type'], 'Point');
      expect(loc['coordinates'], [-73.9654, 40.7829]);
    });
  });

  group('Contract: realtime SSE event', () {
    test('RealtimeEvent matches server SSE data shape', () {
      // Canonical fixture: realtime_event.json
      final json = <String, Object?>{
        'action': 'UPDATE',
        'table': 'posts',
        'record': {
          'id': 'rec_1',
          'title': 'after',
        },
        'oldRecord': {
          'id': 'rec_1',
          'title': 'before',
        },
      };

      final event = RealtimeEvent.fromJson(json);
      expect(event.action, 'UPDATE');
      expect(event.table, 'posts');
      expect(event.record['id'], 'rec_1');
      expect(event.oldRecord?['title'], 'before');
    });
  });
}
