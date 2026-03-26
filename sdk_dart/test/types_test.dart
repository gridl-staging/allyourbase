import 'package:allyourbase/allyourbase.dart';
import 'package:test/test.dart';

void main() {
  group('User', () {
    test('fromJson/toJson round-trip', () {
      final user = User.fromJson(const {
        'id': 'usr_123',
        'email': 'dev@example.com',
        'emailVerified': true,
        'createdAt': '2026-02-22T00:00:00Z',
        'updatedAt': '2026-02-22T01:00:00Z',
      });

      expect(user.id, 'usr_123');
      expect(user.email, 'dev@example.com');
      expect(user.emailVerified, isTrue);
      expect(user.createdAt, '2026-02-22T00:00:00Z');
      expect(user.updatedAt, '2026-02-22T01:00:00Z');

      expect(user.toJson(), {
        'id': 'usr_123',
        'email': 'dev@example.com',
        'emailVerified': true,
        'createdAt': '2026-02-22T00:00:00Z',
        'updatedAt': '2026-02-22T01:00:00Z',
      });
    });
  });

  group('AuthResponse', () {
    test('fromJson parses user payload', () {
      final auth = AuthResponse.fromJson(const {
        'token': 'tok_abc',
        'refreshToken': 'rt_abc',
        'user': {
          'id': 'usr_123',
          'email': 'dev@example.com',
        },
      });

      expect(auth.token, 'tok_abc');
      expect(auth.refreshToken, 'rt_abc');
      expect(auth.user.id, 'usr_123');
      expect(auth.user.email, 'dev@example.com');
    });
  });

  group('ListResponse', () {
    test('fromJson decodes generic item list', () {
      final response = ListResponse<Map<String, Object?>>.fromJson(
        const {
          'items': [
            {'id': '1', 'name': 'Alpha'},
            {'id': '2', 'name': 'Beta'},
          ],
          'page': 1,
          'perPage': 20,
          'totalItems': 2,
          'totalPages': 1,
        },
        decodeItem: (value) => Map<String, Object?>.from(
          value as Map<Object?, Object?>,
        ),
      );

      expect(response.items, hasLength(2));
      expect(response.items.first['name'], 'Alpha');
      expect(response.page, 1);
      expect(response.perPage, 20);
      expect(response.totalItems, 2);
      expect(response.totalPages, 1);
    });
  });

  group('RealtimeEvent', () {
    test('fromJson parses action/table/record', () {
      final event = RealtimeEvent.fromJson(const {
        'action': 'create',
        'table': 'posts',
        'record': {'id': 'post_1', 'title': 'Hello'},
      });

      expect(event.action, 'create');
      expect(event.table, 'posts');
      expect(event.record['title'], 'Hello');
    });
  });

  group('StorageObject', () {
    test('fromJson parses optional fields', () {
      final object = StorageObject.fromJson(const {
        'id': 'obj_1',
        'bucket': 'avatars',
        'name': 'a.png',
        'size': 123,
        'contentType': 'image/png',
        'userId': 'usr_1',
        'createdAt': '2026-02-22T00:00:00Z',
        'updatedAt': '2026-02-22T01:00:00Z',
      });

      expect(object.id, 'obj_1');
      expect(object.bucket, 'avatars');
      expect(object.name, 'a.png');
      expect(object.size, 123);
      expect(object.contentType, 'image/png');
      expect(object.userId, 'usr_1');
      expect(object.createdAt, '2026-02-22T00:00:00Z');
      expect(object.updatedAt, '2026-02-22T01:00:00Z');
    });
  });

  group('BatchOperation', () {
    test('toJson emits method/id/body', () {
      final operation = BatchOperation(
        method: 'update',
        id: 'rec_1',
        body: const {'name': 'Updated'},
      );

      expect(operation.toJson(), {
        'method': 'update',
        'id': 'rec_1',
        'body': {'name': 'Updated'},
      });
    });

    test('fromJson reads optional id/body', () {
      final operation = BatchOperation.fromJson(const {
        'method': 'delete',
        'id': 'rec_1',
      });

      expect(operation.method, 'delete');
      expect(operation.id, 'rec_1');
      expect(operation.body, isNull);
    });
  });

  group('BatchResult', () {
    test('fromJson decodes generic body when present', () {
      final result = BatchResult<Map<String, Object?>>.fromJson(
        const {
          'index': 0,
          'status': 200,
          'body': {'id': 'rec_1', 'name': 'Alpha'},
        },
        decodeBody: (value) => Map<String, Object?>.from(
          value as Map<Object?, Object?>,
        ),
      );

      expect(result.index, 0);
      expect(result.status, 200);
      expect(result.body?['name'], 'Alpha');
    });

    test('fromJson keeps body null when absent', () {
      final result = BatchResult<Object>.fromJson(
        const {
          'index': 1,
          'status': 204,
        },
      );

      expect(result.index, 1);
      expect(result.status, 204);
      expect(result.body, isNull);
    });
  });

  group('DeviceToken', () {
    test('fromJson parses push device token payload', () {
      final token = DeviceToken.fromJson(const {
        'id': 'dev_1',
        'provider': 'fcm',
        'platform': 'android',
        'token': 'fcm-token-abc',
        'device_name': 'Pixel 9',
        'is_active': true,
        'last_refreshed_at': '2026-02-22T02:00:00Z',
        'created_at': '2026-02-22T00:00:00Z',
      });

      expect(token.id, 'dev_1');
      expect(token.provider, 'fcm');
      expect(token.platform, 'android');
      expect(token.token, 'fcm-token-abc');
      expect(token.deviceName, 'Pixel 9');
      expect(token.isActive, isTrue);
      expect(token.lastRefreshedAt, '2026-02-22T02:00:00Z');
      expect(token.createdAt, '2026-02-22T00:00:00Z');
    });

    test('toJson includes nullable device name', () {
      final token = DeviceToken(
        id: 'dev_1',
        provider: 'apns',
        platform: 'ios',
        token: 'apns-token',
        isActive: false,
        createdAt: '2026-02-22T00:00:00Z',
      );

      expect(token.toJson()['device_name'], isNull);
      expect(token.toJson()['provider'], 'apns');
      expect(token.toJson()['platform'], 'ios');
      expect(token.toJson()['is_active'], isFalse);
    });
  });

  group('Validation', () {
    test('throws when int fields contain fractional numbers', () {
      expect(
        () => StorageObject.fromJson(const {
          'id': 'obj_1',
          'bucket': 'avatars',
          'name': 'a.png',
          'size': 12.5,
          'contentType': 'image/png',
          'createdAt': '2026-02-22T00:00:00Z',
          'updatedAt': '2026-02-22T01:00:00Z',
        }),
        throwsFormatException,
      );
    });

    test('throws when object keys are non-string', () {
      expect(
        () => RealtimeEvent.fromJson({
          'action': 'create',
          'table': 'posts',
          'record': {1: 'bad-key-type'},
        }),
        throwsFormatException,
      );
    });
  });
}
