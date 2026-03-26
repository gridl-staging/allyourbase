@TestOn('!browser')
library;

import 'package:allyourbase/allyourbase.dart';
import 'package:test/test.dart';

import 'support/integration_env.dart';

final _env = IntegrationTestEnv.fromEnvironment();

void main() {
  group(
    'Integration: auth smoke',
    skip: _env.skipReason,
    () {
      test(
        'registers, refreshes, logs out, logs in, and deletes account',
        () async {
          final baseUrl = _env.baseUrl;
          expect(baseUrl, isNotNull);

          final client = AYBClient(baseUrl!);
          addTearDown(client.close);

          final credentials = _env.newCredentials('auth smoke');
          var accountDeleted = false;

          try {
            final registered = await client.auth.register(
              credentials.email,
              credentials.password,
            );
            expect(registered.token, isNotEmpty);
            expect(registered.refreshToken, isNotEmpty);
            expect(registered.user.email, credentials.email);
            expect(client.token, isNotEmpty);
            expect(client.refreshToken, isNotEmpty);

            final me = await client.auth.me();
            expect(me.email, credentials.email);

            final refreshed = await client.auth.refresh();
            expect(refreshed.token, isNotEmpty);
            expect(refreshed.refreshToken, isNotEmpty);

            await client.auth.logout();
            expect(client.token, isNull);
            expect(client.refreshToken, isNull);

            final loggedIn = await client.auth.login(
              credentials.email,
              credentials.password,
            );
            expect(loggedIn.user.email, credentials.email);
            expect(client.token, isNotEmpty);
            expect(client.refreshToken, isNotEmpty);

            await client.auth.deleteAccount();
            accountDeleted = true;
            expect(client.token, isNull);
            expect(client.refreshToken, isNull);
          } finally {
            if (!accountDeleted) {
              try {
                if (client.token == null) {
                  await client.auth.login(
                    credentials.email,
                    credentials.password,
                  );
                }
                await client.auth.deleteAccount();
              } catch (_) {
                // Best-effort cleanup to avoid orphaned test users.
              }
            }
          }
        },
        timeout: Timeout(_env.timeout),
      );
    },
  );
}
