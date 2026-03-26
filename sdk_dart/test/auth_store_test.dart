import 'dart:async';
import 'dart:convert';

import 'package:allyourbase/allyourbase.dart';
import 'package:test/test.dart';

void main() {
  group('AuthStore', () {
    test('starts with null tokens', () {
      final store = AuthStore();

      expect(store.token, isNull);
      expect(store.refreshToken, isNull);
      expect(store.isValid, isFalse);
    });

    test('save() stores tokens', () {
      final store = AuthStore();

      store.save('jwt_123', 'refresh_123');

      expect(store.token, 'jwt_123');
      expect(store.refreshToken, 'refresh_123');
    });

    test('clear() removes tokens', () {
      final store = AuthStore();
      store.save('jwt_123', 'refresh_123');

      store.clear();

      expect(store.token, isNull);
      expect(store.refreshToken, isNull);
    });

    test('onChange emits event on save()', () async {
      final store = AuthStore();
      final events = <AuthStoreEvent>[];
      store.onChange.listen(events.add);

      store.save('jwt_123', 'refresh_123');
      // Allow microtask to complete for broadcast stream.
      await Future<void>.delayed(Duration.zero);

      expect(events, hasLength(1));
      expect(events.first.token, 'jwt_123');
      expect(events.first.refreshToken, 'refresh_123');
    });

    test('onChange emits event on clear()', () async {
      final store = AuthStore();
      store.save('jwt_123', 'refresh_123');
      final events = <AuthStoreEvent>[];
      store.onChange.listen(events.add);

      store.clear();
      await Future<void>.delayed(Duration.zero);

      expect(events, hasLength(1));
      expect(events.first.token, isNull);
      expect(events.first.refreshToken, isNull);
    });

    test('onChange is a broadcast stream (multiple listeners)', () async {
      final store = AuthStore();
      final eventsA = <AuthStoreEvent>[];
      final eventsB = <AuthStoreEvent>[];
      store.onChange.listen(eventsA.add);
      store.onChange.listen(eventsB.add);

      store.save('jwt_1', 'refresh_1');
      await Future<void>.delayed(Duration.zero);

      expect(eventsA, hasLength(1));
      expect(eventsB, hasLength(1));
    });

    group('isValid', () {
      test('returns false when no token set', () {
        final store = AuthStore();
        expect(store.isValid, isFalse);
      });

      test('returns true for non-expired JWT', () {
        final store = AuthStore();
        final token = _makeJwt(
          exp: DateTime.now()
                  .add(const Duration(hours: 1))
                  .millisecondsSinceEpoch ~/
              1000,
        );
        store.save(token, 'refresh_123');

        expect(store.isValid, isTrue);
      });

      test('returns false for expired JWT', () {
        final store = AuthStore();
        final token = _makeJwt(
          exp: DateTime.now()
                  .subtract(const Duration(hours: 1))
                  .millisecondsSinceEpoch ~/
              1000,
        );
        store.save(token, 'refresh_123');

        expect(store.isValid, isFalse);
      });

      test('returns false for malformed token (not 3 parts)', () {
        final store = AuthStore();
        store.save('not-a-jwt', 'refresh_123');

        expect(store.isValid, isFalse);
      });

      test('returns false for JWT with invalid base64 payload', () {
        final store = AuthStore();
        store.save('header.!!!invalid!!!.signature', 'refresh_123');

        expect(store.isValid, isFalse);
      });

      test('returns false for JWT without exp claim', () {
        final store = AuthStore();
        final payload =
            base64Url.encode(utf8.encode(jsonEncode({'sub': 'usr_1'})));
        store.save('header.$payload.signature', 'refresh_123');

        expect(store.isValid, isFalse);
      });
    });

    test('dispose() closes the stream controller', () async {
      final store = AuthStore();
      store.dispose();

      // After dispose, listening should complete immediately (stream is done).
      final completer = Completer<void>();
      store.onChange.listen(
        (_) {},
        onDone: () => completer.complete(),
      );
      await completer.future.timeout(
        const Duration(seconds: 1),
        onTimeout: () => fail('Stream did not close after dispose'),
      );
    });
  });

  group('AsyncAuthStore', () {
    test('loads initial tokens from serialized data', () {
      final store = AsyncAuthStore(
        save: (_) async {},
        clear: () async {},
        initial: jsonEncode({
          'token': 'jwt_initial',
          'refreshToken': 'refresh_initial',
        }),
      );

      expect(store.token, 'jwt_initial');
      expect(store.refreshToken, 'refresh_initial');
    });

    test('starts empty when no initial data provided', () {
      final store = AsyncAuthStore(
        save: (_) async {},
        clear: () async {},
      );

      expect(store.token, isNull);
      expect(store.refreshToken, isNull);
    });

    test('save() calls persistence callback with serialized data', () async {
      String? savedData;
      final store = AsyncAuthStore(
        save: (data) async {
          savedData = data;
        },
        clear: () async {},
      );

      store.save('jwt_new', 'refresh_new');

      // Allow async callback to execute.
      await Future<void>.delayed(Duration.zero);
      expect(savedData, isNotNull);
      final decoded = jsonDecode(savedData!) as Map<String, Object?>;
      expect(decoded['token'], 'jwt_new');
      expect(decoded['refreshToken'], 'refresh_new');
    });

    test('clear() calls persistence clear callback', () async {
      var clearCalled = false;
      final store = AsyncAuthStore(
        save: (_) async {},
        clear: () async {
          clearCalled = true;
        },
      );
      store.save('jwt_123', 'refresh_123');

      store.clear();

      await Future<void>.delayed(Duration.zero);
      expect(clearCalled, isTrue);
      expect(store.token, isNull);
      expect(store.refreshToken, isNull);
    });

    test('emits onChange events like base AuthStore', () async {
      final store = AsyncAuthStore(
        save: (_) async {},
        clear: () async {},
      );
      final events = <AuthStoreEvent>[];
      store.onChange.listen(events.add);

      store.save('jwt_1', 'refresh_1');
      await Future<void>.delayed(Duration.zero);

      expect(events, hasLength(1));
      expect(events.first.token, 'jwt_1');
    });

    test('isValid works with initial token', () {
      final token = _makeJwt(
        exp: DateTime.now()
                .add(const Duration(hours: 1))
                .millisecondsSinceEpoch ~/
            1000,
      );
      final store = AsyncAuthStore(
        save: (_) async {},
        clear: () async {},
        initial: jsonEncode({
          'token': token,
          'refreshToken': 'refresh_123',
        }),
      );

      expect(store.isValid, isTrue);
    });
  });
}

/// Create a minimal JWT token with the given exp claim.
String _makeJwt({required int exp}) {
  final header = base64Url.encode(utf8.encode(jsonEncode({'alg': 'HS256'})));
  final payload = base64Url.encode(
    utf8.encode(jsonEncode({'sub': 'usr_1', 'exp': exp})),
  );
  return '$header.$payload.fake_signature';
}
