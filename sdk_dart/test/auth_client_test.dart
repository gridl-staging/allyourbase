import 'package:allyourbase/allyourbase.dart';
import 'package:test/test.dart';

import 'support/deterministic_http_client.dart';

/// Standard auth response fixture matching AYB API shape.
const _authResponseJson = <String, Object?>{
  'token': 'jwt_new',
  'refreshToken': 'refresh_new',
  'user': {
    'id': 'usr_1',
    'email': 'test@example.com',
    'emailVerified': true,
    'createdAt': '2026-01-01T00:00:00Z',
    'updatedAt': '2026-01-01T00:00:00Z',
  },
};

const _userJson = <String, Object?>{
  'id': 'usr_1',
  'email': 'test@example.com',
  'emailVerified': true,
  'createdAt': '2026-01-01T00:00:00Z',
  'updatedAt': '2026-01-01T00:00:00Z',
};

void main() {
  group('AuthClient', () {
    group('register', () {
      test('POSTs email+password to /api/auth/register', () async {
        final http = DeterministicHttpClient([
          StubResponse.json(200, _authResponseJson),
        ]);
        final client = AYBClient('https://api.example.com', httpClient: http);

        await client.auth.register('test@example.com', 'password123');

        final req = http.requests.single;
        expect(req.method, 'POST');
        expect(req.url.toString(), 'https://api.example.com/api/auth/register');
        expect(
          _header(req.headers, 'Content-Type'),
          'application/json',
        );
        expect(req.decodeJsonBody(), {
          'email': 'test@example.com',
          'password': 'password123',
        });
      });

      test('returns parsed AuthResponse', () async {
        final http = DeterministicHttpClient([
          StubResponse.json(200, _authResponseJson),
        ]);
        final client = AYBClient('https://api.example.com', httpClient: http);

        final result =
            await client.auth.register('test@example.com', 'password123');

        expect(result.token, 'jwt_new');
        expect(result.refreshToken, 'refresh_new');
        expect(result.user.id, 'usr_1');
        expect(result.user.email, 'test@example.com');
        expect(result.user.emailVerified, isTrue);
      });

      test('stores tokens on client after success', () async {
        final http = DeterministicHttpClient([
          StubResponse.json(200, _authResponseJson),
        ]);
        final client = AYBClient('https://api.example.com', httpClient: http);

        await client.auth.register('test@example.com', 'password123');

        expect(client.token, 'jwt_new');
        expect(client.refreshToken, 'refresh_new');
      });

      test('emits SIGNED_IN event on success', () async {
        final http = DeterministicHttpClient([
          StubResponse.json(200, _authResponseJson),
        ]);
        final client = AYBClient('https://api.example.com', httpClient: http);
        final events = <String>[];
        client.onAuthStateChange((event, _) => events.add(event));

        await client.auth.register('test@example.com', 'password123');

        expect(events, ['SIGNED_IN']);
      });

      test('propagates AYBError on 409 conflict', () async {
        final http = DeterministicHttpClient([
          StubResponse.json(409, const {
            'message': 'Email already registered',
            'code': 'auth/email-exists',
          }),
        ]);
        final client = AYBClient('https://api.example.com', httpClient: http);

        await expectLater(
          () => client.auth.register('test@example.com', 'password123'),
          throwsA(
            isA<AYBError>()
                .having((e) => e.status, 'status', 409)
                .having((e) => e.message, 'message', 'Email already registered')
                .having((e) => e.code, 'code', 'auth/email-exists'),
          ),
        );
      });
    });

    group('login', () {
      test('POSTs email+password to /api/auth/login', () async {
        final http = DeterministicHttpClient([
          StubResponse.json(200, _authResponseJson),
        ]);
        final client = AYBClient('https://api.example.com', httpClient: http);

        await client.auth.login('test@example.com', 'password123');

        final req = http.requests.single;
        expect(req.method, 'POST');
        expect(req.url.toString(), 'https://api.example.com/api/auth/login');
        expect(req.decodeJsonBody(), {
          'email': 'test@example.com',
          'password': 'password123',
        });
      });

      test('stores tokens and emits SIGNED_IN', () async {
        final http = DeterministicHttpClient([
          StubResponse.json(200, _authResponseJson),
        ]);
        final client = AYBClient('https://api.example.com', httpClient: http);
        final events = <String>[];
        client.onAuthStateChange((event, _) => events.add(event));

        final result =
            await client.auth.login('test@example.com', 'password123');

        expect(result.token, 'jwt_new');
        expect(client.token, 'jwt_new');
        expect(client.refreshToken, 'refresh_new');
        expect(events, ['SIGNED_IN']);
      });

      test('propagates AYBError on 401 bad credentials', () async {
        final http = DeterministicHttpClient([
          StubResponse.json(401, const {
            'message': 'Invalid credentials',
            'code': 'auth/invalid-credentials',
          }),
        ]);
        final client = AYBClient('https://api.example.com', httpClient: http);

        await expectLater(
          () => client.auth.login('test@example.com', 'wrong'),
          throwsA(
            isA<AYBError>()
                .having((e) => e.status, 'status', 401)
                .having((e) => e.message, 'message', 'Invalid credentials'),
          ),
        );
      });
    });

    group('me', () {
      test('GETs /api/auth/me with auth header', () async {
        final http = DeterministicHttpClient([
          StubResponse.json(200, _userJson),
        ]);
        final client = AYBClient('https://api.example.com', httpClient: http);
        client.setTokens('jwt_abc', 'refresh_abc');

        await client.auth.me();

        final req = http.requests.single;
        expect(req.method, 'GET');
        expect(req.url.toString(), 'https://api.example.com/api/auth/me');
        expect(_header(req.headers, 'Authorization'), 'Bearer jwt_abc');
      });

      test('returns parsed User', () async {
        final http = DeterministicHttpClient([
          StubResponse.json(200, _userJson),
        ]);
        final client = AYBClient('https://api.example.com', httpClient: http);
        client.setTokens('jwt_abc', 'refresh_abc');

        final user = await client.auth.me();

        expect(user.id, 'usr_1');
        expect(user.email, 'test@example.com');
        expect(user.emailVerified, isTrue);
      });

      test('does not modify tokens or emit events', () async {
        final http = DeterministicHttpClient([
          StubResponse.json(200, _userJson),
        ]);
        final client = AYBClient('https://api.example.com', httpClient: http);
        client.setTokens('jwt_abc', 'refresh_abc');
        final events = <String>[];
        client.onAuthStateChange((event, _) => events.add(event));

        await client.auth.me();

        expect(client.token, 'jwt_abc');
        expect(client.refreshToken, 'refresh_abc');
        expect(events, isEmpty);
      });
    });

    group('refresh', () {
      test('POSTs refreshToken to /api/auth/refresh', () async {
        final http = DeterministicHttpClient([
          StubResponse.json(200, _authResponseJson),
        ]);
        final client = AYBClient('https://api.example.com', httpClient: http);
        client.setTokens('jwt_old', 'refresh_old');

        await client.auth.refresh();

        final req = http.requests.single;
        expect(req.method, 'POST');
        expect(req.url.toString(), 'https://api.example.com/api/auth/refresh');
        expect(req.decodeJsonBody(), {'refreshToken': 'refresh_old'});
      });

      test('stores new tokens and emits TOKEN_REFRESHED', () async {
        final http = DeterministicHttpClient([
          StubResponse.json(200, _authResponseJson),
        ]);
        final client = AYBClient('https://api.example.com', httpClient: http);
        client.setTokens('jwt_old', 'refresh_old');
        final events = <String>[];
        client.onAuthStateChange((event, _) => events.add(event));

        final result = await client.auth.refresh();

        expect(result.token, 'jwt_new');
        expect(client.token, 'jwt_new');
        expect(client.refreshToken, 'refresh_new');
        expect(events, ['TOKEN_REFRESHED']);
      });

      test('throws when refresh token is missing and skips request', () async {
        final http = DeterministicHttpClient();
        final client = AYBClient('https://api.example.com', httpClient: http);
        client.setApiKey('ayb_key_only');
        final events = <String>[];
        client.onAuthStateChange((event, _) => events.add(event));

        await expectLater(
          () => client.auth.refresh(),
          throwsA(
            isA<AYBError>()
                .having((e) => e.status, 'status', 400)
                .having((e) => e.code, 'code', 'auth/missing-refresh-token')
                .having(
                  (e) => e.message,
                  'message',
                  'Missing refresh token',
                ),
          ),
        );

        expect(http.requests, isEmpty);
        expect(client.token, 'ayb_key_only');
        expect(client.refreshToken, isNull);
        expect(events, isEmpty);
      });

      test('throws when refresh token is empty string and skips request',
          () async {
        final http = DeterministicHttpClient();
        final client = AYBClient('https://api.example.com', httpClient: http);
        client.setTokens('jwt_abc', '');
        final events = <String>[];
        client.onAuthStateChange((event, _) => events.add(event));

        await expectLater(
          () => client.auth.refresh(),
          throwsA(
            isA<AYBError>()
                .having((e) => e.status, 'status', 400)
                .having((e) => e.code, 'code', 'auth/missing-refresh-token'),
          ),
        );

        expect(http.requests, isEmpty);
        expect(events, isEmpty);
      });
    });

    group('logout', () {
      test('POSTs refreshToken to /api/auth/logout', () async {
        final http = DeterministicHttpClient([StubResponse.empty(204)]);
        final client = AYBClient('https://api.example.com', httpClient: http);
        client.setTokens('jwt_abc', 'refresh_abc');

        await client.auth.logout();

        final req = http.requests.single;
        expect(req.method, 'POST');
        expect(req.url.toString(), 'https://api.example.com/api/auth/logout');
        expect(req.decodeJsonBody(), {'refreshToken': 'refresh_abc'});
      });

      test('clears tokens and emits SIGNED_OUT', () async {
        final http = DeterministicHttpClient([StubResponse.empty(204)]);
        final client = AYBClient('https://api.example.com', httpClient: http);
        client.setTokens('jwt_abc', 'refresh_abc');
        final events = <String>[];
        client.onAuthStateChange((event, _) => events.add(event));

        await client.auth.logout();

        expect(client.token, isNull);
        expect(client.refreshToken, isNull);
        expect(events, ['SIGNED_OUT']);
      });

      test('throws when refresh token is missing and skips request', () async {
        final http = DeterministicHttpClient();
        final client = AYBClient('https://api.example.com', httpClient: http);
        client.setApiKey('ayb_key_only');
        final events = <String>[];
        client.onAuthStateChange((event, _) => events.add(event));

        await expectLater(
          () => client.auth.logout(),
          throwsA(
            isA<AYBError>()
                .having((e) => e.status, 'status', 400)
                .having((e) => e.code, 'code', 'auth/missing-refresh-token')
                .having(
                  (e) => e.message,
                  'message',
                  'Missing refresh token',
                ),
          ),
        );

        expect(http.requests, isEmpty);
        expect(client.token, 'ayb_key_only');
        expect(client.refreshToken, isNull);
        expect(events, isEmpty);
      });
    });

    group('deleteAccount', () {
      test('DELETEs /api/auth/me', () async {
        final http = DeterministicHttpClient([StubResponse.empty(204)]);
        final client = AYBClient('https://api.example.com', httpClient: http);
        client.setTokens('jwt_abc', 'refresh_abc');

        await client.auth.deleteAccount();

        final req = http.requests.single;
        expect(req.method, 'DELETE');
        expect(req.url.toString(), 'https://api.example.com/api/auth/me');
        expect(_header(req.headers, 'Authorization'), 'Bearer jwt_abc');
      });

      test('clears tokens and emits SIGNED_OUT', () async {
        final http = DeterministicHttpClient([StubResponse.empty(204)]);
        final client = AYBClient('https://api.example.com', httpClient: http);
        client.setTokens('jwt_abc', 'refresh_abc');
        final events = <String>[];
        client.onAuthStateChange((event, _) => events.add(event));

        await client.auth.deleteAccount();

        expect(client.token, isNull);
        expect(client.refreshToken, isNull);
        expect(events, ['SIGNED_OUT']);
      });
    });

    group('requestPasswordReset', () {
      test('POSTs email to /api/auth/password-reset', () async {
        final http = DeterministicHttpClient([StubResponse.empty(204)]);
        final client = AYBClient('https://api.example.com', httpClient: http);

        await client.auth.requestPasswordReset('test@example.com');

        final req = http.requests.single;
        expect(req.method, 'POST');
        expect(
          req.url.toString(),
          'https://api.example.com/api/auth/password-reset',
        );
        expect(req.decodeJsonBody(), {'email': 'test@example.com'});
      });

      test('does not require auth token', () async {
        final http = DeterministicHttpClient([StubResponse.empty(204)]);
        final client = AYBClient('https://api.example.com', httpClient: http);

        await client.auth.requestPasswordReset('test@example.com');

        expect(_header(http.requests.single.headers, 'Authorization'), isNull);
      });
    });

    group('confirmPasswordReset', () {
      test('POSTs token+password to /api/auth/password-reset/confirm',
          () async {
        final http = DeterministicHttpClient([StubResponse.empty(204)]);
        final client = AYBClient('https://api.example.com', httpClient: http);

        await client.auth.confirmPasswordReset('reset_tok_123', 'newPass456');

        final req = http.requests.single;
        expect(req.method, 'POST');
        expect(
          req.url.toString(),
          'https://api.example.com/api/auth/password-reset/confirm',
        );
        expect(req.decodeJsonBody(), {
          'token': 'reset_tok_123',
          'password': 'newPass456',
        });
      });
    });

    group('verifyEmail', () {
      test('POSTs token to /api/auth/verify', () async {
        final http = DeterministicHttpClient([StubResponse.empty(204)]);
        final client = AYBClient('https://api.example.com', httpClient: http);

        await client.auth.verifyEmail('verify_tok_123');

        final req = http.requests.single;
        expect(req.method, 'POST');
        expect(
          req.url.toString(),
          'https://api.example.com/api/auth/verify',
        );
        expect(req.decodeJsonBody(), {'token': 'verify_tok_123'});
      });
    });

    group('resendVerification', () {
      test('POSTs to /api/auth/verify/resend with auth header', () async {
        final http = DeterministicHttpClient([StubResponse.empty(204)]);
        final client = AYBClient('https://api.example.com', httpClient: http);
        client.setTokens('jwt_abc', 'refresh_abc');

        await client.auth.resendVerification();

        final req = http.requests.single;
        expect(req.method, 'POST');
        expect(
          req.url.toString(),
          'https://api.example.com/api/auth/verify/resend',
        );
        expect(_header(req.headers, 'Authorization'), 'Bearer jwt_abc');
      });
    });

    group('handleOAuthRedirect', () {
      test('parses token+refreshToken from URI fragment', () {
        final client = AYBClient('https://api.example.com');
        final uri = Uri.parse(
          'https://myapp.com/callback#token=jwt_oauth&refreshToken=refresh_oauth',
        );

        final result = client.auth.handleOAuthRedirect(uri);

        expect(result, isNotNull);
        expect(result!.token, 'jwt_oauth');
        expect(result.refreshToken, 'refresh_oauth');
        // User is a placeholder — consumer should call me() for full profile.
        expect(result.user.id, isEmpty);
        expect(result.user.email, isEmpty);
      });

      test('stores tokens and emits SIGNED_IN', () {
        final client = AYBClient('https://api.example.com');
        final events = <String>[];
        client.onAuthStateChange((event, _) => events.add(event));
        final uri = Uri.parse(
          'https://myapp.com/callback#token=jwt_oauth&refreshToken=refresh_oauth',
        );

        client.auth.handleOAuthRedirect(uri);

        expect(client.token, 'jwt_oauth');
        expect(client.refreshToken, 'refresh_oauth');
        expect(events, ['SIGNED_IN']);
      });

      test('returns null when fragment is empty', () {
        final client = AYBClient('https://api.example.com');

        final result =
            client.auth.handleOAuthRedirect(Uri.parse('https://myapp.com/callback'));

        expect(result, isNull);
      });

      test('returns null when token is missing from fragment', () {
        final client = AYBClient('https://api.example.com');
        final uri = Uri.parse(
          'https://myapp.com/callback#refreshToken=refresh_oauth',
        );

        final result = client.auth.handleOAuthRedirect(uri);

        expect(result, isNull);
      });

      test('returns null when refreshToken is missing from fragment', () {
        final client = AYBClient('https://api.example.com');
        final uri = Uri.parse('https://myapp.com/callback#token=jwt_oauth');

        final result = client.auth.handleOAuthRedirect(uri);

        expect(result, isNull);
      });

      // Note: Dart's Uri class normalizes fragments (% → %25), so malformed
      // percent-encoding can't reach handleOAuthRedirect via a real Uri object.
      // The defensive try/catch in the implementation is kept but untestable.

      test('does not modify tokens when fragment is incomplete', () {
        final client = AYBClient('https://api.example.com');
        client.setTokens('existing_jwt', 'existing_refresh');
        final events = <String>[];
        client.onAuthStateChange((event, _) => events.add(event));
        final uri = Uri.parse('https://myapp.com/callback#token=jwt_only');

        client.auth.handleOAuthRedirect(uri);

        expect(client.token, 'existing_jwt');
        expect(client.refreshToken, 'existing_refresh');
        expect(events, isEmpty);
      });
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
